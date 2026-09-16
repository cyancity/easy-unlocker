package broker

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cyancity/easy-unlocker/internal/protocol"
	"github.com/cyancity/easy-unlocker/internal/securepayload"
)

const (
	defaultMaxTTL        = 1 * time.Hour
	defaultNotifyTimeout = 5 * time.Second
	defaultMaxPending    = 256
	maxRequestBody       = 64 << 10
)

type ServerConfig struct {
	PairingToken  string
	AdminToken    string
	Store         SecretStore
	Notifier      Notifier
	Audit         AuditSink
	MaxTTL        time.Duration
	NotifyTimeout time.Duration
	DeviceFile    string
	FCM           *FCMClient
	// Version 是构建时注入的 release tag（vYYYY.MM.DD[-n]），/v1/version 报给 CLI 做更新提示。
	Version       string
}

type Server struct {
	pairingToken  []byte
	adminToken    []byte
	decisionKey   []byte
	store         SecretStore
	notifier      Notifier
	audit         AuditSink
	maxTTL        time.Duration
	notifyTimeout time.Duration
	deviceFile    string
	fcm           *FCMClient
	maxPending    int
	version       string

	mu        sync.Mutex
	pending   map[string]*pendingRequest
	devices   map[string]deviceRecord
	pairCodes map[string]pairCode
	// pendingByKey 按 request_key 索引 pending：断线重连时同 key 续等原请求。
	pendingByKey map[string]*pendingRequest
}

// detachGrace 是断线宽限期：连接断开后 pending 再留这么久等客户端重连，
// 真取消（Ctrl-C 走人）也最多在手机上多挂这一会儿就自清。
const detachGrace = 15 * time.Second

// pairCode 绑生成它的 approver 所在租户：claim 出的 requester 落在同租户。
type pairCode struct {
	expires time.Time
	tenant  string
}

const (
	deviceTTL   = 180 * 24 * time.Hour
	pairCodeTTL = 10 * time.Minute
	// 去掉 0/O/1/I 这类看起来像的字符。
	pairCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

const (
	deviceRoleApprover  = "approver"
	deviceRoleRequester = "requester"
)

type deviceRecord struct {
	ID         string    `json:"id,omitempty"`
	Name       string    `json:"name"`
	Role       string    `json:"role,omitempty"`
	Tenant     string    `json:"tenant,omitempty"`
	Created    time.Time `json:"created"`
	ExpiresAt  time.Time `json:"expires_at,omitempty"`
	LastUsedAt time.Time `json:"last_used_at,omitempty"`
	PushToken  string    `json:"push_token,omitempty"`
}

// tenant 缺省按 default：多租户上线前的老 devices.json 全部归入同一租户，零迁移。
const defaultTenant = "default"

func (d deviceRecord) tenant() string {
	if d.Tenant == "" {
		return defaultTenant
	}
	return d.Tenant
}

// role 缺省按 approver：老 devices.json 分不清手机和 CLI，当 requester 会把在用的手机
// 废掉。已配对的 CLI 要降级成 requester，得重新跑一次 easyGet pair。
func (d deviceRecord) role() string {
	if d.Role == "" {
		return deviceRoleApprover
	}
	return d.Role
}

type pendingRequest struct {
	id         string
	request    protocol.Request
	received   time.Time
	expires    time.Time
	approve    string
	deny       string
	done       chan protocol.Response
	stop       chan struct{}
	state      pendingState
	deviceID   string
	deviceName string
	tenant     string
	// key 是 request.request_key 的缓存：客户端断线重连时按它找回同一条 pending。
	key string
	// detached 标记「客户端连接断了但请求还活着」：进入重连宽限期，超时未续才取消。
	detached bool
}

type pendingState uint8

const (
	pendingStateWaiting pendingState = iota
	pendingStateDeciding
)

type PendingView struct {
	RequestID     string    `json:"request_id"`
	Item          string    `json:"item"`
	Mode          string    `json:"mode"`
	Purpose       string    `json:"purpose"`
	TTL           int       `json:"ttl"`
	Target        string    `json:"target,omitempty"`
	Requester     string    `json:"requester,omitempty"`
	Delivery      string    `json:"delivery,omitempty"`
	Received      time.Time `json:"received_at"`
	Expires       time.Time `json:"expires_at"`
	State         string    `json:"state"`
	SealPublicKey string    `json:"seal_public_key,omitempty"`
	// sign 请求的三个字段：手机要看到 public_key/ssh_user/cert_ttl 才能签发证书。
	PublicKey  string `json:"public_key,omitempty"`
	SSHUser    string `json:"ssh_user,omitempty"`
	CertTTL    int    `json:"cert_ttl,omitempty"`
	DeviceID   string `json:"device_id,omitempty"`
	DeviceName string `json:"device_name,omitempty"`
}

func NewServer(config ServerConfig) (*Server, error) {
	if strings.TrimSpace(config.PairingToken) == "" {
		return nil, errors.New("pairing token is required")
	}
	if config.Store == nil {
		return nil, errors.New("secret store is required")
	}
	if config.Audit == nil {
		config.Audit = noopAudit{}
	}
	if config.MaxTTL <= 0 {
		config.MaxTTL = defaultMaxTTL
	}
	if config.NotifyTimeout <= 0 {
		config.NotifyTimeout = defaultNotifyTimeout
	}
	decisionKey := make([]byte, 32)
	if _, err := rand.Read(decisionKey); err != nil {
		return nil, errors.New("cannot initialize decision signer")
	}
	s := &Server{
		pairingToken:  []byte(config.PairingToken),
		adminToken:    []byte(config.AdminToken),
		decisionKey:   decisionKey,
		store:         config.Store,
		notifier:      config.Notifier,
		audit:         config.Audit,
		maxTTL:        config.MaxTTL,
		notifyTimeout: config.NotifyTimeout,
		deviceFile:    strings.TrimSpace(config.DeviceFile),
		fcm:           config.FCM,
		version:       config.Version,
		maxPending:    defaultMaxPending,
		pending:       make(map[string]*pendingRequest),
		pendingByKey:  make(map[string]*pendingRequest),
		pairCodes:     make(map[string]pairCode),
		devices:       make(map[string]deviceRecord),
	}
	s.loadDevices()
	return s, nil
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(s.serveHTTP)
}

// ClosePending wakes every long-poll with a non-credential failure. It is
// used during shutdown so a Broker outage cannot leave a caller waiting until
// the original TTL.
func (s *Server) ClosePending() {
	s.mu.Lock()
	requests := make([]*pendingRequest, 0, len(s.pending))
	for id, pending := range s.pending {
		delete(s.pending, id)
		close(pending.stop)
		requests = append(requests, pending)
	}
	s.mu.Unlock()
	for _, pending := range requests {
		_ = s.auditRequest(pending, "broker_shutdown", protocol.StatusFailed, "broker shutdown")
		pending.done <- protocol.Response{Status: protocol.StatusFailed, RequestID: pending.id, Message: "Broker shutting down"}
	}
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/v1/version":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, protocol.Response{Status: protocol.StatusFailed, Message: "method not allowed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"version": s.version})
	case r.URL.Path == "/healthz":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, protocol.Response{Status: protocol.StatusFailed, Message: "method not allowed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case r.URL.Path == "/v1/request":
		s.handleRequest(w, r)
	case r.URL.Path == "/v1/admin/pending":
		s.handlePending(w, r)
	case r.URL.Path == "/v1/device/pair":
		s.handleDevicePair(w, r)
	case r.URL.Path == "/v1/device/pair-code":
		s.handleDevicePairCode(w, r)
	case r.URL.Path == "/v1/pair/claim":
		s.handlePairClaim(w, r)
	case r.URL.Path == "/v1/device/devices":
		s.handleDeviceList(w, r)
	case r.URL.Path == "/v1/device/revoke":
		s.handleDeviceRevoke(w, r)
	case r.URL.Path == "/v1/device/rename":
		s.handleDeviceRename(w, r)
	case r.URL.Path == "/v1/device/renew":
		s.handleDeviceRenew(w, r)
	case r.URL.Path == "/v1/device/pending":
		s.handleDevicePending(w, r)
	case r.URL.Path == "/v1/device/push-token":
		s.handleDevicePushToken(w, r)
	case strings.HasPrefix(r.URL.Path, "/v1/decision/"):
		s.handleDecision(w, r)
	default:
		writeJSON(w, http.StatusNotFound, protocol.Response{Status: protocol.StatusFailed, Message: "not found"})
	}
}

func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, protocol.Response{Status: protocol.StatusFailed, Message: "method not allowed"})
		return
	}
	auth := r.Header.Get("Authorization")
	pairingOK := s.authorizedBearer(auth, s.pairingToken)
	var device deviceRecord
	deviceOK := false
	if !pairingOK {
		device, deviceOK = s.deviceAuthorized(auth)
	}
	// 配对后的设备令牌与全局 pairing token 都能发起请求：配对的目的就是让新机器能取凭据。
	if !pairingOK && !deviceOK {
		writeJSON(w, http.StatusUnauthorized, protocol.Response{Status: "unauthorized", Message: "authorization required"})
		return
	}
	var request protocol.Request
	if err := decodeJSON(r, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, protocol.Response{Status: "invalid_request", Message: "invalid request body"})
		return
	}
	maxTTLSeconds := int(s.maxTTL / time.Second)
	if err := request.Validate(maxTTLSeconds); err != nil {
		writeJSON(w, http.StatusBadRequest, protocol.Response{Status: "invalid_request", Message: "invalid request fields"})
		return
	}
	if request.Requester == "" {
		request.Requester = "unknown"
	}
	// 租户从发起方令牌反查：requester 配在哪个租户，请求就属于哪个租户；
	// pairing token 直发的归入 default。CLI 不传租户 id，伪造不了。
	tenant := device.tenant()

	// 幂等续等：客户端断线（代理掐长连接/网络抖动）后带同 request_key 重发，
	// 同租户找回原 pending 继续等，手机上不会出现第二条。
	key := strings.TrimSpace(request.RequestKey)
	if key != "" {
		s.mu.Lock()
		if prev, ok := s.pendingByKey[key]; ok && prev.tenant == tenant && prev.state == pendingStateWaiting {
			prev.detached = false
			s.mu.Unlock()
			_ = s.auditRequest(prev, "request_reattached", "", "client reconnected")
			s.waitResult(w, r, prev)
			return
		}
		s.mu.Unlock()
	}

	now := time.Now().UTC()
	id, err := s.newRequestID()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, protocol.Response{Status: protocol.StatusFailed, Message: "cannot create request"})
		return
	}
	pending := &pendingRequest{
		id:         id,
		request:    request,
		received:   now,
		expires:    now.Add(time.Duration(request.TTL) * time.Second),
		approve:    s.decisionSignature(id, "approve"),
		deny:       s.decisionSignature(id, "deny"),
		done:       make(chan protocol.Response, 1),
		stop:       make(chan struct{}),
		deviceID:   device.ID,
		deviceName: device.Name,
		tenant:     tenant,
		key:        key,
	}
	s.mu.Lock()
	if len(s.pending) >= s.maxPending {
		s.mu.Unlock()
		writeJSON(w, http.StatusServiceUnavailable, protocol.Response{Status: protocol.StatusFailed, Message: "too many pending requests"})
		return
	}
	s.pending[id] = pending
	if key != "" {
		s.pendingByKey[key] = pending
	}
	s.mu.Unlock()

	if err := s.auditRequest(pending, "request_created", "", ""); err != nil {
		s.removePending(id)
		writeJSON(w, http.StatusServiceUnavailable, protocol.Response{Status: protocol.StatusFailed, Message: "audit log unavailable"})
		return
	}
	go s.expireRequest(pending)
	if s.fcm != nil || s.notifier != nil {
		notifyCtx, cancel := context.WithTimeout(context.Background(), s.notifyTimeout)
		notifyErr := s.push(notifyCtx, pending.tenant, Notification{
			RequestID:  id,
			ApproveSig: pending.approve,
			DenySig:    pending.deny,
		})
		cancel()
		if notifyErr != nil {
			s.finish(pending, protocol.Response{Status: protocol.StatusFailed, RequestID: id, Message: "notification unavailable"}, "notification_failed", "notification unavailable")
		}
	}

	s.waitResult(w, r, pending)
}

// waitResult 把新建和幂等续等两条路汇合：等到决策或客户端断开。
// 断开不再是取消——进 detach 宽限期，给重连留窗口（见 detachPending）。
func (s *Server) waitResult(w http.ResponseWriter, r *http.Request, pending *pendingRequest) {
	select {
	case response := <-pending.done:
		writeJSON(w, responseHTTPStatus(response.Status), response)
	case <-r.Context().Done():
		s.detachPending(pending)
	}
}

func (s *Server) handleDecision(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, protocol.Response{Status: protocol.StatusFailed, Message: "method not allowed"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/decision/")
	if id == "" || strings.Contains(id, "/") {
		writeJSON(w, http.StatusNotFound, protocol.Response{Status: protocol.StatusFailed, Message: "request not found"})
		return
	}
	var decision protocol.Decision
	if err := decodeJSON(r, &decision); err != nil {
		writeJSON(w, http.StatusBadRequest, protocol.Response{Status: "invalid_decision", Message: "invalid decision body"})
		return
	}
	if decision.Decision != "approve" && decision.Decision != "deny" {
		writeJSON(w, http.StatusBadRequest, protocol.Response{Status: "invalid_decision", Message: "invalid decision"})
		return
	}
	pending, ok := s.claimDecision(id, decision.Decision, decision.Sig, r.Header.Get("Authorization"), decision.Payload)
	if !ok {
		writeJSON(w, http.StatusForbidden, protocol.Response{Status: "invalid_decision", Message: "invalid or already used decision"})
		return
	}
	if decision.Decision == "deny" {
		s.finish(pending, protocol.Response{Status: protocol.StatusDenied, RequestID: id}, "decision", "deny")
	} else if strings.TrimSpace(pending.request.SealPublicKey) != "" {
		if strings.TrimSpace(decision.Payload) == "" || !strings.HasPrefix(decision.Payload, "v2.") {
			s.finish(pending, protocol.Response{Status: protocol.StatusFailed, RequestID: id, Message: "phone payload required"}, "decision", "missing phone payload")
		} else {
			s.finish(pending, protocol.Response{
				Status:    protocol.StatusApproved,
				Mode:      pending.request.Mode,
				Payload:   decision.Payload,
				RequestID: id,
			}, "decision", "approve")
		}
	} else {
		go s.approve(pending)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
}

func (s *Server) handlePending(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, protocol.Response{Status: protocol.StatusFailed, Message: "method not allowed"})
		return
	}
	if len(s.adminToken) == 0 || !s.authorizedBearer(r.Header.Get("X-Admin-Token"), s.adminToken) {
		writeJSON(w, http.StatusUnauthorized, protocol.Response{Status: "unauthorized", Message: "admin authorization required"})
		return
	}
	s.mu.Lock()
	views := make([]PendingView, 0, len(s.pending))
	for _, pending := range s.pending {
		state := "waiting"
		if pending.state == pendingStateDeciding {
			state = "deciding"
		}
		views = append(views, s.pendingView(pending, state))
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, views)
}

func (s *Server) approve(pending *pendingRequest) {
	if time.Now().UTC().After(pending.expires) {
		s.finish(pending, protocol.Response{Status: protocol.StatusExpired, RequestID: pending.id}, "decision", "expired")
		return
	}
	request := pending.request
	var value []byte
	var err error
	switch request.Mode {
	case protocol.ModeWrite:
		if request.Target == "" {
			s.finish(pending, protocol.Response{Status: protocol.StatusFailed, RequestID: pending.id, Message: "write target is required"}, "decision", "missing target")
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		value, err = s.store.Get(ctx, request.Item)
		cancel()
	default:
		// sign 没有服务端取值路径：证书只能由手机签发密封回来（见 protocol.Validate）。
		err = errors.New("unsupported mode")
	}
	if err != nil {
		clearBytes(value)
		s.finish(pending, protocol.Response{Status: protocol.StatusFailed, RequestID: pending.id, Message: "credential release failed"}, "decision", "credential release failed")
		return
	}
	if time.Now().UTC().After(pending.expires) {
		clearBytes(value)
		s.finish(pending, protocol.Response{Status: protocol.StatusExpired, RequestID: pending.id}, "decision", "expired")
		return
	}
	payload, err := securepayload.Seal(string(s.pairingToken), pending.id, value)
	clearBytes(value)
	if err != nil {
		s.finish(pending, protocol.Response{Status: protocol.StatusFailed, RequestID: pending.id, Message: "credential encryption failed"}, "decision", "credential encryption failed")
		return
	}
	s.finish(pending, protocol.Response{
		Status:    protocol.StatusApproved,
		Mode:      request.Mode,
		Payload:   payload,
		RequestID: pending.id,
	}, "decision", "approve")
}

func (s *Server) expireRequest(pending *pendingRequest) {
	timer := time.NewTimer(time.Until(pending.expires))
	defer timer.Stop()
	select {
	case <-timer.C:
		s.mu.Lock()
		if current, ok := s.pending[pending.id]; ok && current.state == pendingStateWaiting {
			s.dropPendingLocked(pending)
			s.mu.Unlock()
			s.sendResult(pending, protocol.Response{Status: protocol.StatusExpired, RequestID: pending.id}, "expired", "ttl elapsed")
			return
		}
		s.mu.Unlock()
	case <-pending.stop:
		return
	}
}

func (s *Server) claimDecision(id, decision, signature, authorization, payload string) (*pendingRequest, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pending, ok := s.pending[id]
	if !ok || pending.state != pendingStateWaiting || time.Now().UTC().After(pending.expires) {
		return nil, false
	}
	expected := pending.approve
	if decision == "deny" {
		expected = pending.deny
	}
	sigOK := signature != "" && hmac.Equal([]byte(expected), []byte(signature))
	// 设备令牌决定只能批自己租户的请求；FCM sig 本来就绑定单个 request_id，
	// 别的租户手机拿不到 sig，天然进不来。
	deviceOK := s.approverAuthorizedTenantLocked(authorization, pending.tenant)
	if !sigOK && !deviceOK {
		return nil, false
	}
	if decision == "approve" && strings.TrimSpace(pending.request.SealPublicKey) != "" && strings.TrimSpace(payload) == "" && !sigOK {
		// device approve of a phone-sealed request must include the box payload
		return nil, false
	}
	pending.state = pendingStateDeciding
	return pending, true
}

func (s *Server) pendingView(pending *pendingRequest, state string) PendingView {
	return PendingView{
		RequestID:     pending.id,
		Item:          pending.request.Item,
		Mode:          pending.request.Mode,
		Purpose:       pending.request.Purpose,
		TTL:           pending.request.TTL,
		Target:        pending.request.Target,
		Requester:     pending.request.Requester,
		Delivery:      pending.request.Delivery,
		Received:      pending.received,
		Expires:       pending.expires,
		State:         state,
		SealPublicKey: pending.request.SealPublicKey,
		PublicKey:     pending.request.PublicKey,
		SSHUser:       pending.request.SSHUser,
		CertTTL:       pending.request.CertTTL,
		DeviceID:      pending.deviceID,
		DeviceName:    pending.deviceName,
	}
}

func (s *Server) handleDevicePair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, protocol.Response{Status: protocol.StatusFailed, Message: "method not allowed"})
		return
	}
	if !s.authorizedBearer(r.Header.Get("Authorization"), s.pairingToken) {
		writeJSON(w, http.StatusUnauthorized, protocol.Response{Status: "unauthorized", Message: "authorization required"})
		return
	}
	var body struct {
		Name    string `json:"name"`
		VaultID string `json:"vault_id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		body.Name = ""
		body.VaultID = ""
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "phone"
	}
	if len(name) > 64 {
		name = name[:64]
	}
	tenant := strings.TrimSpace(body.VaultID)
	if len(tenant) > 64 {
		tenant = tenant[:64]
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, protocol.Response{Status: protocol.StatusFailed, Message: "cannot create device token"})
		return
	}
	token := hex.EncodeToString(raw)
	s.mu.Lock()
	device := s.newDeviceLocked(name, deviceRoleApprover)
	device.Tenant = tenant
	// 独占语义：同一租户同时只有一台 active approver——新手机配对上岗即把
	// 同租户其它 approver 撤销（换机踢下线）。requester 不受影响。
	for otherToken, other := range s.devices {
		if other.role() == deviceRoleApprover && other.tenant() == device.tenant() {
			delete(s.devices, otherToken)
		}
	}
	s.devices[token] = device
	err := s.saveDevicesLocked()
	s.mu.Unlock()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, protocol.Response{Status: protocol.StatusFailed, Message: "cannot persist device token"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"device_token": token, "name": name})
}

func (s *Server) loadDevices() {
	if s.deviceFile == "" {
		return
	}
	raw, err := os.ReadFile(s.deviceFile)
	if err != nil {
		return
	}
	var dump map[string]deviceRecord
	if json.Unmarshal(raw, &dump) != nil || dump == nil {
		return
	}
	// 老文件没有 id / expires_at：补齐，避免老设备永远不过期、也没法被列表与撤销。
	now := time.Now().UTC()
	for token, rec := range dump {
		if rec.ID == "" {
			id := make([]byte, 4)
			if _, err := rand.Read(id); err != nil {
				return
			}
			rec.ID = hex.EncodeToString(id)
		}
		if rec.ExpiresAt.IsZero() {
			rec.ExpiresAt = now.Add(deviceTTL)
		}
		dump[token] = rec
	}
	s.devices = dump
	if err := s.saveDevicesLocked(); err != nil {
		return
	}
}

// ---------- 配对码：已授权设备生成，新机器一次性兑换成设备令牌 ----------

func (s *Server) handleDevicePairCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, protocol.Response{Status: protocol.StatusFailed, Message: "method not allowed"})
		return
	}
	s.mu.Lock()
	approver, approverOK := s.authorizedDeviceLocked(r.Header.Get("Authorization"))
	if !approverOK || approver.role() != deviceRoleApprover {
		s.mu.Unlock()
		writeJSON(w, http.StatusUnauthorized, protocol.Response{Status: "unauthorized", Message: "approver device required"})
		return
	}
	now := time.Now().UTC()
	for code, pc := range s.pairCodes {
		if pc.expires.Before(now) {
			delete(s.pairCodes, code)
		}
	}
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		s.mu.Unlock()
		writeJSON(w, http.StatusServiceUnavailable, protocol.Response{Status: protocol.StatusFailed, Message: "cannot create pair code"})
		return
	}
	code := make([]byte, 0, len(raw))
	for _, b := range raw {
		code = append(code, pairCodeAlphabet[int(b)%len(pairCodeAlphabet)])
	}
	expires := now.Add(pairCodeTTL)
	s.pairCodes[string(code)] = pairCode{expires: expires, tenant: approver.tenant()}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"code": string(code), "expires_at": expires.Format(time.RFC3339)})
}

// 无鉴权：配对码本身就是凭证（一次性、10 分钟）。先删码再换令牌，并发兑换只有一个成功；
// 失败一律同一条消息，不区分不存在/过期/已用过，避免被当探针。
func (s *Server) handlePairClaim(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, protocol.Response{Status: protocol.StatusFailed, Message: "method not allowed"})
		return
	}
	var body struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.Code) == "" {
		writeJSON(w, http.StatusBadRequest, protocol.Response{Status: "invalid_request", Message: "code required"})
		return
	}
	code := strings.ToUpper(strings.TrimSpace(body.Code))
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "device"
	}
	if len(name) > 64 {
		name = name[:64]
	}
	s.mu.Lock()
	pc, ok := s.pairCodes[code]
	if !ok {
		s.mu.Unlock()
		writeJSON(w, http.StatusForbidden, protocol.Response{Status: "invalid_claim", Message: "invalid or expired code"})
		return
	}
	delete(s.pairCodes, code)
	if pc.expires.Before(time.Now().UTC()) {
		s.mu.Unlock()
		writeJSON(w, http.StatusForbidden, protocol.Response{Status: "invalid_claim", Message: "invalid or expired code"})
		return
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		s.mu.Unlock()
		writeJSON(w, http.StatusServiceUnavailable, protocol.Response{Status: protocol.StatusFailed, Message: "cannot create device token"})
		return
	}
	// 必须走 hex：原始随机字节当 map key / JSON 字符串时会因非法 UTF-8 被替换，
	// CLI 拿到的令牌就和 broker 存的对不上，之后全部 401。
	token := hex.EncodeToString(raw)
	device := s.newDeviceLocked(name, deviceRoleRequester)
	// requester 继承配对码绑定的租户：CLI 从 pair-code 换令牌起就固定归
	// 发码那台手机所在的租户，之后请求都路由给它。
	device.Tenant = pc.tenant
	s.devices[token] = device
	err := s.saveDevicesLocked()
	s.mu.Unlock()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, protocol.Response{Status: protocol.StatusFailed, Message: "cannot persist device"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"device_token": string(token),
		"name":         device.Name,
		"expires_at":   device.ExpiresAt.Format(time.RFC3339),
	})
}

func (s *Server) newDeviceLocked(name, role string) deviceRecord {
	now := time.Now().UTC()
	id := make([]byte, 4)
	if _, err := rand.Read(id); err != nil {
		id = []byte("--------")
	}
	return deviceRecord{ID: hex.EncodeToString(id), Name: name, Role: role, Created: now, ExpiresAt: now.Add(deviceTTL)}
}

// 设备列表：不下发令牌本体，只给 id 与元数据，便于 App 展示与撤销。
func (s *Server) handleDeviceList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, protocol.Response{Status: protocol.StatusFailed, Message: "method not allowed"})
		return
	}
	s.mu.Lock()
	caller, callerOK := s.authorizedDeviceLocked(r.Header.Get("Authorization"))
	if !callerOK || caller.role() != deviceRoleApprover {
		s.mu.Unlock()
		writeJSON(w, http.StatusUnauthorized, protocol.Response{Status: "unauthorized", Message: "approver device required"})
		return
	}
	current := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	now := time.Now().UTC()
	devices := make([]map[string]any, 0, len(s.devices))
	for token, rec := range s.devices {
		if rec.ExpiresAt.Before(now) || rec.tenant() != caller.tenant() {
			continue
		}
		item := map[string]any{
			"id":         rec.ID,
			"name":       rec.Name,
			"role":       rec.role(),
			"created_at": rec.Created.Format(time.RFC3339),
			"expires_at": rec.ExpiresAt.Format(time.RFC3339),
			"current":    token == current,
		}
		if !rec.LastUsedAt.IsZero() {
			item["last_used_at"] = rec.LastUsedAt.Format(time.RFC3339)
		}
		devices = append(devices, item)
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"devices": devices})
}

// 撤销设备。允许撤销自己——这就是「登出这台设备」；否则最后一台永远删不掉。
func (s *Server) handleDeviceRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, protocol.Response{Status: protocol.StatusFailed, Message: "method not allowed"})
		return
	}
	s.mu.Lock()
	caller, callerOK := s.authorizedDeviceLocked(r.Header.Get("Authorization"))
	if !callerOK || caller.role() != deviceRoleApprover {
		s.mu.Unlock()
		writeJSON(w, http.StatusUnauthorized, protocol.Response{Status: "unauthorized", Message: "approver device required"})
		return
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.ID) == "" {
		s.mu.Unlock()
		writeJSON(w, http.StatusBadRequest, protocol.Response{Status: "invalid_request", Message: "id required"})
		return
	}
	removed := 0
	// 只能撤自己租户的设备；别的租户的 id 存在也当不存在（removed=0）。
	for token, rec := range s.devices {
		if rec.ID == body.ID && rec.tenant() == caller.tenant() {
			delete(s.devices, token)
			removed = 1
			break
		}
	}
	err := s.saveDevicesLocked()
	s.mu.Unlock()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, protocol.Response{Status: protocol.StatusFailed, Message: "cannot persist devices"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "removed": removed})
}

// 改名设备。同租户规则同撤销：别的租户的 id 存在也当不存在。
func (s *Server) handleDeviceRename(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, protocol.Response{Status: protocol.StatusFailed, Message: "method not allowed"})
		return
	}
	s.mu.Lock()
	caller, callerOK := s.authorizedDeviceLocked(r.Header.Get("Authorization"))
	if !callerOK || caller.role() != deviceRoleApprover {
		s.mu.Unlock()
		writeJSON(w, http.StatusUnauthorized, protocol.Response{Status: "unauthorized", Message: "approver device required"})
		return
	}
	var body struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.ID) == "" || strings.TrimSpace(body.Name) == "" {
		s.mu.Unlock()
		writeJSON(w, http.StatusBadRequest, protocol.Response{Status: "invalid_request", Message: "id and name required"})
		return
	}
	name := strings.TrimSpace(body.Name)
	renamed := 0
	for token, rec := range s.devices {
		if rec.ID == body.ID && rec.tenant() == caller.tenant() {
			rec.Name = name
			s.devices[token] = rec
			renamed = 1
			break
		}
	}
	err := s.saveDevicesLocked()
	s.mu.Unlock()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, protocol.Response{Status: protocol.StatusFailed, Message: "cannot persist devices"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "renamed": renamed})
}

func (s *Server) handleDeviceRenew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, protocol.Response{Status: protocol.StatusFailed, Message: "method not allowed"})
		return
	}
	s.mu.Lock()
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	rec, ok := s.authorizedDeviceLocked(r.Header.Get("Authorization"))
	if !ok || rec.role() != deviceRoleApprover {
		s.mu.Unlock()
		writeJSON(w, http.StatusUnauthorized, protocol.Response{Status: "unauthorized", Message: "approver device required"})
		return
	}
	rec.ExpiresAt = time.Now().UTC().Add(deviceTTL)
	rec.LastUsedAt = time.Now().UTC()
	s.devices[token] = rec
	err := s.saveDevicesLocked()
	s.mu.Unlock()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, protocol.Response{Status: protocol.StatusFailed, Message: "cannot persist devices"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "expires_at": rec.ExpiresAt.Format(time.RFC3339)})
}

func (s *Server) saveDevicesLocked() error {
	if s.deviceFile == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.deviceFile), 0o700); err != nil && !os.IsExist(err) {
		if filepath.Dir(s.deviceFile) != "." {
			return err
		}
	}
	raw, err := json.Marshal(s.devices)
	if err != nil {
		return err
	}
	tmp := s.deviceFile + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.deviceFile)
}

func (s *Server) handleDevicePending(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, protocol.Response{Status: protocol.StatusFailed, Message: "method not allowed"})
		return
	}
	s.mu.Lock()
	caller, callerOK := s.authorizedDeviceLocked(r.Header.Get("Authorization"))
	if !callerOK || caller.role() != deviceRoleApprover {
		s.mu.Unlock()
		writeJSON(w, http.StatusUnauthorized, protocol.Response{Status: "unauthorized", Message: "approver device required"})
		return
	}
	views := make([]PendingView, 0, len(s.pending))
	for _, pending := range s.pending {
		if pending.tenant != caller.tenant() {
			continue
		}
		state := "waiting"
		if pending.state == pendingStateDeciding {
			state = "deciding"
		}
		views = append(views, s.pendingView(pending, state))
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, views)
}

func (s *Server) handleDevicePushToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, protocol.Response{Status: protocol.StatusFailed, Message: "method not allowed"})
		return
	}
	auth := r.Header.Get("Authorization")
	var body struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.Token) == "" {
		writeJSON(w, http.StatusBadRequest, protocol.Response{Status: "invalid_request", Message: "push token required"})
		return
	}
	s.mu.Lock()
	if !s.approverAuthorizedLocked(auth) {
		s.mu.Unlock()
		writeJSON(w, http.StatusUnauthorized, protocol.Response{Status: "unauthorized", Message: "approver device required"})
		return
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	rec := s.devices[token]
	rec.PushToken = strings.TrimSpace(body.Token)
	s.devices[token] = rec
	err := s.saveDevicesLocked()
	s.mu.Unlock()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, protocol.Response{Status: protocol.StatusFailed, Message: "cannot persist push token"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) push(ctx context.Context, tenant string, notification Notification) error {
	sent := false
	if s.fcm != nil {
		s.mu.Lock()
		var tokens []string
		for _, device := range s.devices {
			if device.role() == deviceRoleApprover && device.tenant() == tenant && strings.TrimSpace(device.PushToken) != "" {
				tokens = append(tokens, device.PushToken)
			}
		}
		s.mu.Unlock()
		for _, token := range tokens {
			if err := s.fcm.Send(ctx, token, notification); err == nil {
				sent = true
			}
		}
	}
	if sent {
		return nil
	}
	if s.notifier != nil {
		return s.notifier.Notify(ctx, notification)
	}
	return nil
}

// authorizedDeviceLocked 解析 Bearer 设备令牌：存在、未过期才算有效；顺手节流记 lastUsedAt（1 小时）。
func (s *Server) authorizedDeviceLocked(authorization string) (deviceRecord, bool) {
	if !strings.HasPrefix(authorization, "Bearer ") {
		return deviceRecord{}, false
	}
	token := strings.TrimPrefix(authorization, "Bearer ")
	if token == "" || strings.ContainsAny(token, " \t\r\n") {
		return deviceRecord{}, false
	}
	rec, ok := s.devices[token]
	if !ok {
		return deviceRecord{}, false
	}
	now := time.Now().UTC()
	if !rec.ExpiresAt.IsZero() && now.After(rec.ExpiresAt) {
		return deviceRecord{}, false
	}
	if rec.LastUsedAt.IsZero() || now.Sub(rec.LastUsedAt) > time.Hour {
		rec.LastUsedAt = now
		s.devices[token] = rec
		_ = s.saveDevicesLocked()
	}
	return rec, true
}

// approverAuthorizedLocked 额外要求 approver 角色：批准、设备管理都走它。
func (s *Server) approverAuthorizedLocked(authorization string) bool {
	rec, ok := s.authorizedDeviceLocked(authorization)
	return ok && rec.role() == deviceRoleApprover
}

// approverAuthorizedTenantLocked 进一步要求 approver 属于指定租户：
// 设备令牌路径的决定权按租户收。
func (s *Server) approverAuthorizedTenantLocked(authorization, tenant string) bool {
	rec, ok := s.authorizedDeviceLocked(authorization)
	return ok && rec.role() == deviceRoleApprover && rec.tenant() == tenant
}

// deviceAuthorized 是带锁版本，只管「是不是合法设备」、不看角色：/v1/request 用它。
func (s *Server) deviceAuthorized(authorization string) (deviceRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.authorizedDeviceLocked(authorization)
}

func (s *Server) finish(pending *pendingRequest, response protocol.Response, event, reason string) {
	s.mu.Lock()
	current, ok := s.pending[pending.id]
	if ok && current == pending {
		s.dropPendingLocked(pending)
	}
	s.mu.Unlock()
	if !ok || current != pending {
		return
	}
	if response.Status == protocol.StatusApproved {
		if err := s.auditRequest(pending, event, response.Status, reason); err != nil {
			response.Payload = ""
			response = protocol.Response{Status: protocol.StatusFailed, RequestID: pending.id, Message: "audit log unavailable"}
		}
	} else {
		_ = s.auditRequest(pending, event, response.Status, reason)
	}
	pending.done <- response
}

func (s *Server) sendResult(pending *pendingRequest, response protocol.Response, event, reason string) {
	_ = s.auditRequest(pending, event, response.Status, reason)
	pending.done <- response
}

// dropPendingLocked 统一删 pending 的两个索引（id 与 request_key），并停掉过期定时器。
// 调用方必须持 s.mu。
func (s *Server) dropPendingLocked(pending *pendingRequest) {
	delete(s.pending, pending.id)
	if pending.key != "" {
		delete(s.pendingByKey, pending.key)
	}
	close(pending.stop)
}

// detachPending 标记客户端断线但不立刻删：挂着的请求再等 detachGrace 给重连
// 机会。宽限期内带同 request_key 重发会清掉 detached 标志续上；超时才真取消，
// 手机上的待批准条目随之消失。
func (s *Server) detachPending(pending *pendingRequest) {
	s.mu.Lock()
	if current, ok := s.pending[pending.id]; !ok || current != pending || pending.state != pendingStateWaiting {
		s.mu.Unlock()
		return
	}
	pending.detached = true
	s.mu.Unlock()
	_ = s.auditRequest(pending, "request_detached", "", "client disconnected")
	time.AfterFunc(detachGrace, func() {
		s.mu.Lock()
		if current, ok := s.pending[pending.id]; ok && current == pending && pending.detached && pending.state == pendingStateWaiting {
			s.dropPendingLocked(pending)
			s.mu.Unlock()
			_ = s.auditRequest(pending, "request_cancelled", "cancelled", "client detached past grace")
			return
		}
		s.mu.Unlock()
	})
}

func (s *Server) removePending(id string) {
	s.mu.Lock()
	if pending, ok := s.pending[id]; ok {
		s.dropPendingLocked(pending)
	}
	s.mu.Unlock()
}

func (s *Server) auditRequest(pending *pendingRequest, event, result, reason string) error {
	device := pending.deviceID
	if device == "" {
		device = "pairing-token"
	}
	return s.audit.Record(AuditEvent{
		Event:     event,
		RequestID: pending.id,
		Item:      pending.request.Item,
		Mode:      pending.request.Mode,
		Requester: pending.request.Requester,
		Target:    pending.request.Target,
		Device:    device,
		Tenant:    pending.tenant,
		Result:    result,
		Decision:  reason,
	})
}

func (s *Server) newRequestID() (string, error) {
	for attempt := 0; attempt < 4; attempt++ {
		raw := make([]byte, 16)
		if _, err := rand.Read(raw); err != nil {
			return "", errors.New("cannot generate request ID")
		}
		id := hex.EncodeToString(raw)
		s.mu.Lock()
		_, exists := s.pending[id]
		s.mu.Unlock()
		if !exists {
			return id, nil
		}
	}
	return "", errors.New("cannot allocate request ID")
}

func (s *Server) decisionSignature(id, decision string) string {
	mac := hmac.New(sha256.New, s.decisionKey)
	_, _ = mac.Write([]byte("easy-unlocker/v1/decision/"))
	_, _ = mac.Write([]byte(id))
	_, _ = mac.Write([]byte("/"))
	_, _ = mac.Write([]byte(decision))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Server) authorizedBearer(value string, expected []byte) bool {
	if len(expected) == 0 || !strings.HasPrefix(value, "Bearer ") {
		return false
	}
	provided := strings.TrimPrefix(value, "Bearer ")
	if provided == "" || strings.ContainsAny(provided, " \t\r\n") {
		return false
	}
	return hmac.Equal([]byte(provided), expected)
}

func decodeJSON(r *http.Request, destination any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxRequestBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("multiple JSON values")
	}
	return nil
}

func responseHTTPStatus(status string) int {
	switch status {
	case protocol.StatusApproved:
		return http.StatusOK
	case protocol.StatusDenied:
		return http.StatusForbidden
	case protocol.StatusExpired:
		return http.StatusRequestTimeout
	case protocol.StatusFailed:
		return http.StatusServiceUnavailable
	default:
		return http.StatusBadRequest
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func clearBytes(value []byte) {
	for i := range value {
		value[i] = 0
	}
}

package broker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cyancity/easy-unlocker/internal/boxpayload"
	"github.com/cyancity/easy-unlocker/internal/protocol"
	"github.com/cyancity/easy-unlocker/internal/securepayload"
)

type testAudit struct {
	mu     sync.Mutex
	events []AuditEvent
}

func (a *testAudit) Record(event AuditEvent) error {
	a.mu.Lock()
	a.events = append(a.events, event)
	a.mu.Unlock()
	return nil
}

type testNotifier struct {
	ch  chan Notification
	err error
}

func (n *testNotifier) Notify(_ context.Context, notification Notification) error {
	if n.err != nil {
		return n.err
	}
	n.ch <- notification
	return nil
}

func newTestServer(t *testing.T, store SecretStore, notifier Notifier, audit AuditSink, maxTTL time.Duration) (*Server, *httptest.Server) {
	t.Helper()
	server, err := NewServer(ServerConfig{
		PairingToken: "pairing-test-token",
		AdminToken:   "admin-test-token",
		Store:        store,
		Notifier:     notifier,
		Audit:        audit,
		MaxTTL:       maxTTL,
		Version:      "v2026.09.16",
	})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	return server, httpServer
}

func doJSON(t *testing.T, method, url, bearer string, body any) (*http.Response, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	httpRequest, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if bearer != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+bearer)
	}
	if body != nil {
		httpRequest.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response, data
}

// pairApprover 走 pairing-token 的 /v1/device/pair，拿到一张 approver 设备令牌（手机路径）。
func pairApprover(t *testing.T, url string) string {
	t.Helper()
	response, body := doJSON(t, http.MethodPost, url+"/v1/device/pair", "pairing-test-token", map[string]string{"name": "phone"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("pair approver status=%d body=%s", response.StatusCode, body)
	}
	var result struct {
		DeviceToken string `json:"device_token"`
	}
	if err := json.Unmarshal(body, &result); err != nil || result.DeviceToken == "" {
		t.Fatalf("pair approver body=%s", body)
	}
	return result.DeviceToken
}

// claimRequester 让 approver 生成配对码再兑换，拿到一张 requester 设备令牌（CLI 路径）。
func claimRequester(t *testing.T, url, approverToken string) string {
	t.Helper()
	response, body := doJSON(t, http.MethodPost, url+"/v1/device/pair-code", approverToken, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("pair-code status=%d body=%s", response.StatusCode, body)
	}
	var codeResult struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(body, &codeResult); err != nil || codeResult.Code == "" {
		t.Fatalf("pair-code body=%s", body)
	}
	response, body = doJSON(t, http.MethodPost, url+"/v1/pair/claim", "", map[string]string{"code": codeResult.Code, "name": "requester-cli"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("claim status=%d body=%s", response.StatusCode, body)
	}
	var result struct {
		DeviceToken string `json:"device_token"`
	}
	if err := json.Unmarshal(body, &result); err != nil || result.DeviceToken == "" {
		t.Fatalf("claim body=%s", body)
	}
	return result.DeviceToken
}

func postRequest(t *testing.T, endpoint string, token string, request protocol.Request) (*http.Response, protocol.Response) {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	httpRequest, err := http.NewRequest(http.MethodPost, endpoint+"/v1/request", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+token)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result protocol.Response
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return response, result
}

func TestVersionEndpointIsPublic(t *testing.T) {
	_, httpServer := newTestServer(t, NewMapStore(nil), nil, &testAudit{}, 30*time.Second)
	response, body := doJSON(t, http.MethodGet, httpServer.URL+"/v1/version", "", nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("version endpoint should be public, got %d", response.StatusCode)
	}
	var result struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &result); err != nil || result.Version != "v2026.09.16" {
		t.Fatalf("expected build version, got %s", body)
	}
	response, _ = doJSON(t, http.MethodPost, httpServer.URL+"/v1/version", "", nil)
	if response.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST should be 405, got %d", response.StatusCode)
	}
}

func TestRequestRejectsMissingTokenAndExpires(t *testing.T) {
	audit := &testAudit{}
	_, httpServer := newTestServer(t, NewMapStore(nil), nil, audit, 5*time.Second)
	request := protocol.Request{Item: "test", Mode: protocol.ModeWrite, Purpose: "冒烟", TTL: 1}
	response, result := postRequest(t, httpServer.URL, "", request)
	if response.StatusCode != http.StatusUnauthorized || result.Status != "unauthorized" {
		t.Fatalf("missing token: status=%d result=%+v", response.StatusCode, result)
	}
	response, result = postRequest(t, httpServer.URL, "pairing-test-token", request)
	if response.StatusCode != http.StatusRequestTimeout || result.Status != protocol.StatusExpired {
		t.Fatalf("expiry: status=%d result=%+v", response.StatusCode, result)
	}
	if result.RequestID == "" {
		t.Fatal("expired response did not include request ID")
	}
	if len(audit.events) != 2 {
		t.Fatalf("expected create and expiry audit events, got %d", len(audit.events))
	}
}

func TestDecisionSignatureIsSingleUseAndPayloadIsEncrypted(t *testing.T) {
	const secret = "fixture-value-never-in-response"
	audit := &testAudit{}
	notifier := &testNotifier{ch: make(chan Notification, 1)}
	_, httpServer := newTestServer(t, NewMapStore(map[string][]byte{"OPENAI_API_KEY": []byte(secret + "\n")}), notifier, audit, 30*time.Second)
	request := protocol.Request{
		Item:      "OPENAI_API_KEY",
		Mode:      protocol.ModeWrite,
		Purpose:   "集成测试",
		TTL:       10,
		Target:    "/tmp/test.env",
		Requester: "agent@test",
	}
	resultCh := make(chan protocol.Response, 1)
	go func() {
		_, result := postRequest(t, httpServer.URL, "pairing-test-token", request)
		resultCh <- result
	}()
	notification := <-notifier.ch
	if notification.ApproveSig == "" || notification.DenySig == "" || notification.ApproveSig == notification.DenySig {
		t.Fatal("decision signatures were not distinct")
	}
	invalidResponse, _ := postDecision(t, httpServer.URL, notification.RequestID, "approve", "not-a-valid-signature")
	if invalidResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("invalid signature status=%d", invalidResponse.StatusCode)
	}
	approvedResponse, approvedBody := postDecision(t, httpServer.URL, notification.RequestID, "approve", notification.ApproveSig)
	if approvedResponse.StatusCode != http.StatusOK || approvedBody.Status != "accepted" {
		t.Fatalf("decision callback: status=%d body=%+v", approvedResponse.StatusCode, approvedBody)
	}
	replayResponse, _ := postDecision(t, httpServer.URL, notification.RequestID, "approve", notification.ApproveSig)
	if replayResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("replayed signature status=%d", replayResponse.StatusCode)
	}
	result := <-resultCh
	if result.Status != protocol.StatusApproved || result.Payload == "" {
		t.Fatalf("approved result=%+v", result)
	}
	if strings.Contains(result.Payload, secret) {
		t.Fatal("plaintext secret appeared in Broker response")
	}
	value, err := securepayload.Open("pairing-test-token", result.RequestID, result.Payload)
	if err != nil || string(value) != secret+"\n" {
		t.Fatalf("cannot open encrypted payload: err=%v length=%d", err, len(value))
	}
	for _, event := range audit.events {
		encoded, _ := json.Marshal(event)
		if strings.Contains(string(encoded), secret) {
			t.Fatal("plaintext secret appeared in audit event")
		}
	}
}

func TestPairingTokenCannotReadPendingQueue(t *testing.T) {
	notifier := &testNotifier{ch: make(chan Notification, 1)}
	_, httpServer := newTestServer(t, NewMapStore(nil), notifier, &testAudit{}, 30*time.Second)
	request := protocol.Request{Item: "test", Mode: protocol.ModeWrite, Purpose: "queue", TTL: 5, Target: "/tmp/test"}
	done := make(chan struct{})
	go func() {
		_, _ = postRequest(t, httpServer.URL, "pairing-test-token", request)
		close(done)
	}()
	firstNotification := <-notifier.ch
	adminRequest, err := http.NewRequest(http.MethodGet, httpServer.URL+"/v1/admin/pending", nil)
	if err != nil {
		t.Fatal(err)
	}
	adminRequest.Header.Set("X-Admin-Token", "Bearer pairing-test-token")
	response, err := http.DefaultClient.Do(adminRequest)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("pairing token read status=%d", response.StatusCode)
	}
	adminRequest.Header.Set("X-Admin-Token", "Bearer admin-test-token")
	response, err = http.DefaultClient.Do(adminRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("admin pending status=%d", response.StatusCode)
	}
	var pending []PendingView
	if err := json.NewDecoder(response.Body).Decode(&pending); err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].Item != "test" {
		t.Fatalf("pending=%+v", pending)
	}
	_, _ = postDecision(t, httpServer.URL, firstNotification.RequestID, "deny", firstNotification.DenySig)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not finish after deny")
	}
}

func TestConcurrentRequestsHaveIndependentDecisions(t *testing.T) {
	notifier := &testNotifier{ch: make(chan Notification, 2)}
	_, httpServer := newTestServer(t, NewMapStore(nil), notifier, &testAudit{}, 30*time.Second)
	requests := []protocol.Request{
		{Item: "one", Mode: protocol.ModeWrite, Purpose: "first", TTL: 10, Target: "/tmp/one"},
		{Item: "two", Mode: protocol.ModeWrite, Purpose: "second", TTL: 10, Target: "/tmp/two"},
	}
	results := make(chan protocol.Response, len(requests))
	for _, request := range requests {
		request := request
		go func() {
			_, result := postRequest(t, httpServer.URL, "pairing-test-token", request)
			results <- result
		}()
	}
	notifications := []Notification{<-notifier.ch, <-notifier.ch}
	// 两条请求各自独立决策：第一条拒绝，第二条批准（Notification 里不再带条目名）。
	for i, notification := range notifications {
		decision := "approve"
		signature := notification.ApproveSig
		if i == 0 {
			decision = "deny"
			signature = notification.DenySig
		}
		response, _ := postDecision(t, httpServer.URL, notification.RequestID, decision, signature)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("decision for %s status=%d", notification.RequestID, response.StatusCode)
		}
	}
	statuses := make(map[string]bool)
	for range requests {
		result := <-results
		statuses[result.Status] = true
	}
	if !statuses[protocol.StatusDenied] || !statuses[protocol.StatusFailed] {
		t.Fatalf("independent results=%v", statuses)
	}
}

// postRequestCtx 与 postRequest 同构但带 ctx：模拟客户端中途断开（代理掐长连接）。
func postRequestCtx(ctx context.Context, t *testing.T, endpoint string, token string, request protocol.Request) (protocol.Response, error) {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/v1/request", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+token)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		return protocol.Response{}, err
	}
	defer response.Body.Close()
	var result protocol.Response
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return protocol.Response{}, err
	}
	return result, nil
}

func pendingCount(t *testing.T, s *Server) int {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pending)
}

func TestRequestKeyReattach(t *testing.T) {
	server, httpServer := newTestServer(t, NewMapStore(map[string][]byte{"k": []byte("v")}), nil, &testAudit{}, 30*time.Second)
	approver := pairApprover(t, httpServer.URL)

	request := protocol.Request{
		Item: "k", Mode: protocol.ModeWrite, Purpose: "x", TTL: 25,
		Requester: "t", Target: "/tmp/x", RequestKey: "reattach-key-1",
	}

	// 第一条长轮询挂起
	ctx1, cancel1 := context.WithCancel(context.Background())
	go func() { _, _ = postRequestCtx(ctx1, t, httpServer.URL, "pairing-test-token", request) }()

	deadline := time.Now().Add(3 * time.Second)
	var pendingID string
	for time.Now().Before(deadline) {
		server.mu.Lock()
		for id := range server.pending {
			pendingID = id
		}
		server.mu.Unlock()
		if pendingID != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pendingID == "" {
		t.Fatal("request never became pending")
	}

	// 客户端断开（代理掐线）：服务端标 detached 而不是取消
	cancel1()
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		server.mu.Lock()
		detached := server.pending[pendingID] != nil && server.pending[pendingID].detached
		server.mu.Unlock()
		if detached {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	server.mu.Lock()
	isDetached := server.pending[pendingID] != nil && server.pending[pendingID].detached
	server.mu.Unlock()
	if !isDetached {
		t.Fatal("disconnect should mark pending detached, not cancel it")
	}

	// 同 key 重发：续上原 pending，不产生第二条
	second := make(chan protocol.Response, 1)
	go func() {
		result, err := postRequestCtx(context.Background(), t, httpServer.URL, "pairing-test-token", request)
		if err == nil {
			second <- result
		}
	}()
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		server.mu.Lock()
		stillOne := len(server.pending) == 1 && !server.pending[pendingID].detached
		server.mu.Unlock()
		if stillOne {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pendingCount(t, server) != 1 {
		t.Fatalf("reattach must reuse the pending, got %d", pendingCount(t, server))
	}

	// approver 批准：第二条连接拿到原 request_id 的结果
	response, _ := doJSON(t, http.MethodPost, httpServer.URL+"/v1/decision/"+pendingID, approver,
		map[string]string{"decision": "approve"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("approve status=%d", response.StatusCode)
	}
	select {
	case result := <-second:
		if result.Status != protocol.StatusApproved || result.RequestID != pendingID {
			t.Fatalf("reattached result=%+v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reattached request never got the decision")
	}
}

func TestRequestKeyTenantIsolation(t *testing.T) {
	_, httpServer := newTestServer(t, NewMapStore(map[string][]byte{"k": []byte("v")}), nil, &testAudit{}, 30*time.Second)
	approverA := pairApprover(t, httpServer.URL) // default 租户（pair 没带 vault_id）

	// A 租户的 requester 发带 key 的请求
	requesterA := claimRequester(t, httpServer.URL, approverA)
	request := protocol.Request{
		Item: "k", Mode: protocol.ModeWrite, Purpose: "x", TTL: 20,
		RequestKey: "shared-key",
	}
	first := make(chan protocol.Response, 1)
	go func() {
		r, err := postRequestCtx(context.Background(), t, httpServer.URL, requesterA, request)
		if err == nil {
			first <- r
		}
	}()

	// 等它进 pending，记下 A 的 request_id
	deadline := time.Now().Add(3 * time.Second)
	var idA string
	for {
		_, listBody := doJSON(t, http.MethodGet, httpServer.URL+"/v1/device/pending", approverA, nil)
		var list []PendingView
		_ = json.Unmarshal(listBody, &list)
		if len(list) > 0 {
			idA = list[0].RequestID
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("tenant A request never pending")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// B 租户 approver + requester，用同一个 key 发请求——不能撞上 A 的 pending
	approverB := pairApproverVault(t, httpServer.URL, "vault-b")
	requesterB := claimRequester(t, httpServer.URL, approverB)
	second := make(chan protocol.Response, 1)
	go func() {
		r, err := postRequestCtx(context.Background(), t, httpServer.URL, requesterB, request)
		if err == nil {
			second <- r
		}
	}()

	deadline = time.Now().Add(3 * time.Second)
	var idB string
	for {
		_, listBody := doJSON(t, http.MethodGet, httpServer.URL+"/v1/device/pending", approverB, nil)
		var list []PendingView
		_ = json.Unmarshal(listBody, &list)
		if len(list) == 1 {
			idB = list[0].RequestID
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("tenant B request never pending")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if idB == idA {
		t.Fatal("cross-tenant request_key must not reattach: B got A's request_id")
	}

	// 各批各的，互不相干
	doJSON(t, http.MethodPost, httpServer.URL+"/v1/decision/"+idA, approverA, map[string]string{"decision": "deny"})
	doJSON(t, http.MethodPost, httpServer.URL+"/v1/decision/"+idB, approverB, map[string]string{"decision": "deny"})
	gotA, gotB := <-first, <-second
	if gotA.Status != protocol.StatusDenied || gotB.Status != protocol.StatusDenied {
		t.Fatalf("both should be denied, got A=%s B=%s", gotA.Status, gotB.Status)
	}
}

func TestNotificationFailureFailsClosed(t *testing.T) {
	notifier := &testNotifier{err: errors.New("push unavailable")}
	_, httpServer := newTestServer(t, NewMapStore(nil), notifier, &testAudit{}, 30*time.Second)
	request := protocol.Request{Item: "test", Mode: protocol.ModeWrite, Purpose: "notification failure", TTL: 10, Target: "/tmp/test"}
	response, result := postRequest(t, httpServer.URL, "pairing-test-token", request)
	if response.StatusCode != http.StatusServiceUnavailable || result.Status != protocol.StatusFailed {
		t.Fatalf("notification failure: status=%d result=%+v", response.StatusCode, result)
	}
	adminRequest, err := http.NewRequest(http.MethodGet, httpServer.URL+"/v1/admin/pending", nil)
	if err != nil {
		t.Fatal(err)
	}
	adminRequest.Header.Set("X-Admin-Token", "Bearer admin-test-token")
	adminResponse, err := http.DefaultClient.Do(adminRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer adminResponse.Body.Close()
	var pending []PendingView
	if err := json.NewDecoder(adminResponse.Body).Decode(&pending); err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("failed notification left pending request: %+v", pending)
	}
}

func TestClosePendingWakesLongPollAsFailure(t *testing.T) {
	notifier := &testNotifier{ch: make(chan Notification, 1)}
	server, httpServer := newTestServer(t, NewMapStore(nil), notifier, &testAudit{}, 30*time.Second)
	request := protocol.Request{Item: "test", Mode: protocol.ModeWrite, Purpose: "shutdown", TTL: 20, Target: "/tmp/test"}
	resultCh := make(chan protocol.Response, 1)
	go func() {
		_, result := postRequest(t, httpServer.URL, "pairing-test-token", request)
		resultCh <- result
	}()
	<-notifier.ch
	server.ClosePending()
	select {
	case result := <-resultCh:
		if result.Status != protocol.StatusFailed {
			t.Fatalf("shutdown result status=%q", result.Status)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not wake long poll")
	}
}

func TestPhoneSealedApproveDoesNotUseBrokerStore(t *testing.T) {
	const secret = "only-on-phone"
	notifier := &testNotifier{ch: make(chan Notification, 1)}
	store := NewMapStore(map[string][]byte{"OPENAI_API_KEY": []byte("store-must-not-be-read")})
	_, httpServer := newTestServer(t, store, notifier, &testAudit{}, 30*time.Second)

	recipient, err := boxpayload.Generate()
	if err != nil {
		t.Fatal(err)
	}
	defer recipient.Clear()

	request := protocol.Request{
		Item:          "OPENAI_API_KEY",
		Mode:          protocol.ModeWrite,
		Purpose:       "P1",
		TTL:           10,
		Target:        "/tmp/test.env",
		Requester:     "agent@test",
		SealPublicKey: recipient.PublicBase64(),
	}
	resultCh := make(chan protocol.Response, 1)
	go func() {
		_, result := postRequest(t, httpServer.URL, "pairing-test-token", request)
		resultCh <- result
	}()
	notification := <-notifier.ch
	if notification.RequestID == "" {
		t.Fatal("sealed request should still notify the phone")
	}

	pairBody, _ := json.Marshal(map[string]string{"name": "pixel"})
	pairReq, err := http.NewRequest(http.MethodPost, httpServer.URL+"/v1/device/pair", bytes.NewReader(pairBody))
	if err != nil {
		t.Fatal(err)
	}
	pairReq.Header.Set("Authorization", "Bearer pairing-test-token")
	pairReq.Header.Set("Content-Type", "application/json")
	pairResp, err := http.DefaultClient.Do(pairReq)
	if err != nil {
		t.Fatal(err)
	}
	var pair struct {
		DeviceToken string `json:"device_token"`
	}
	if err := json.NewDecoder(pairResp.Body).Decode(&pair); err != nil {
		t.Fatal(err)
	}
	pairResp.Body.Close()
	if pair.DeviceToken == "" {
		t.Fatal("missing device token")
	}

	pendingReq, err := http.NewRequest(http.MethodGet, httpServer.URL+"/v1/device/pending", nil)
	if err != nil {
		t.Fatal(err)
	}
	pendingReq.Header.Set("Authorization", "Bearer "+pair.DeviceToken)
	pendingResp, err := http.DefaultClient.Do(pendingReq)
	if err != nil {
		t.Fatal(err)
	}
	var views []PendingView
	if err := json.NewDecoder(pendingResp.Body).Decode(&views); err != nil {
		t.Fatal(err)
	}
	pendingResp.Body.Close()
	if len(views) != 1 || views[0].SealPublicKey != recipient.PublicBase64() {
		t.Fatalf("pending=%+v", views)
	}

	envelope, err := boxpayload.Seal(recipient.Public, notification.RequestID, []byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	decisionBody, err := json.Marshal(protocol.Decision{Decision: "approve", Payload: envelope})
	if err != nil {
		t.Fatal(err)
	}
	decReq, err := http.NewRequest(http.MethodPost, httpServer.URL+"/v1/decision/"+notification.RequestID, bytes.NewReader(decisionBody))
	if err != nil {
		t.Fatal(err)
	}
	decReq.Header.Set("Authorization", "Bearer "+pair.DeviceToken)
	decReq.Header.Set("Content-Type", "application/json")
	decResp, err := http.DefaultClient.Do(decReq)
	if err != nil {
		t.Fatal(err)
	}
	if decResp.StatusCode != http.StatusOK {
		t.Fatalf("device approve status=%d", decResp.StatusCode)
	}
	decResp.Body.Close()

	result := <-resultCh
	if result.Status != protocol.StatusApproved {
		t.Fatalf("result=%+v", result)
	}
	if strings.Contains(result.Payload, secret) || strings.Contains(result.Payload, "store-must-not-be-read") {
		t.Fatal("plaintext appeared in broker payload")
	}
	opened, err := boxpayload.Open(recipient, result.RequestID, result.Payload)
	if err != nil || string(opened) != secret {
		t.Fatalf("open box: err=%v value=%q", err, opened)
	}
}

func postDecision(t *testing.T, endpoint, requestID, decision, signature string) (*http.Response, protocol.Response) {
	t.Helper()
	body, err := json.Marshal(protocol.Decision{Decision: decision, Sig: signature})
	if err != nil {
		t.Fatal(err)
	}
	httpRequest, err := http.NewRequest(http.MethodPost, endpoint+"/v1/decision/"+requestID, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var result protocol.Response
	if err := json.Unmarshal(responseBody, &result); err != nil {
		t.Fatal(err)
	}
	return response, result
}

// delivery 只是给手机展示的，Broker 原样透传到待批准视图。
func TestPendingViewCarriesDelivery(t *testing.T) {
	notifier := &testNotifier{ch: make(chan Notification, 1)}
	_, httpServer := newTestServer(t, NewMapStore(nil), notifier, &testAudit{}, 30*time.Second)
	request := protocol.Request{
		Item:     "oci-api-key",
		Mode:     protocol.ModeWrite,
		Purpose:  "不落盘",
		TTL:      5,
		Delivery: protocol.DeliveryEphemeral,
	}
	done := make(chan struct{})
	go func() {
		_, _ = postRequest(t, httpServer.URL, "pairing-test-token", request)
		close(done)
	}()
	firstNotification := <-notifier.ch
	pending := adminPending(t, httpServer.URL)
	if len(pending) != 1 {
		t.Fatalf("pending=%+v", pending)
	}
	if pending[0].Delivery != protocol.DeliveryEphemeral {
		t.Fatalf("delivery=%q, want %q", pending[0].Delivery, protocol.DeliveryEphemeral)
	}
	_, _ = postDecision(t, httpServer.URL, firstNotification.RequestID, "deny", firstNotification.DenySig)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not finish after deny")
	}
}

func TestRequestRejectsUnknownDelivery(t *testing.T) {
	notifier := &testNotifier{ch: make(chan Notification, 1)}
	_, httpServer := newTestServer(t, NewMapStore(nil), notifier, &testAudit{}, 30*time.Second)
	request := protocol.Request{
		Item:     "x",
		Mode:     protocol.ModeWrite,
		Purpose:  "p",
		TTL:      5,
		Target:   "/tmp/x",
		Delivery: "stdout",
	}
	response, _ := postRequest(t, httpServer.URL, "pairing-test-token", request)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d, want %d", response.StatusCode, http.StatusBadRequest)
	}
}

func adminPending(t *testing.T, endpoint string) []PendingView {
	t.Helper()
	adminRequest, err := http.NewRequest(http.MethodGet, endpoint+"/v1/admin/pending", nil)
	if err != nil {
		t.Fatal(err)
	}
	adminRequest.Header.Set("X-Admin-Token", "Bearer admin-test-token")
	response, err := http.DefaultClient.Do(adminRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("admin pending status=%d", response.StatusCode)
	}
	var pending []PendingView
	if err := json.NewDecoder(response.Body).Decode(&pending); err != nil {
		t.Fatal(err)
	}
	return pending
}

func TestExpiredDeviceTokenIsRejected(t *testing.T) {
	server, httpServer := newTestServer(t, NewMapStore(nil), nil, &testAudit{}, 30*time.Second)
	token := pairApprover(t, httpServer.URL)
	server.mu.Lock()
	record := server.devices[token]
	record.ExpiresAt = time.Now().UTC().Add(-time.Hour)
	server.devices[token] = record
	server.mu.Unlock()

	response, _ := doJSON(t, http.MethodGet, httpServer.URL+"/v1/device/pending", token, nil)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired device pending status=%d", response.StatusCode)
	}
	request := protocol.Request{Item: "x", Mode: protocol.ModeWrite, Purpose: "p", TTL: 1, Target: "/tmp/x"}
	response, result := postRequest(t, httpServer.URL, token, request)
	if response.StatusCode != http.StatusUnauthorized || result.Status != "unauthorized" {
		t.Fatalf("expired device request status=%d result=%+v", response.StatusCode, result)
	}
}

func TestRequesterDeviceCannotActAsApprover(t *testing.T) {
	notifier := &testNotifier{ch: make(chan Notification, 2)}
	_, httpServer := newTestServer(t, NewMapStore(map[string][]byte{"KEY": []byte("server-secret")}), notifier, &testAudit{}, 30*time.Second)
	approver := pairApprover(t, httpServer.URL)
	requester := claimRequester(t, httpServer.URL, approver)

	// requester 可以发起请求：这条自然过期（408），不是 401。
	request := protocol.Request{Item: "KEY", Mode: protocol.ModeWrite, Purpose: "requester can ask", TTL: 1, Target: "/tmp/x"}
	response, result := postRequest(t, httpServer.URL, requester, request)
	if response.StatusCode != http.StatusRequestTimeout || result.Status != protocol.StatusExpired {
		t.Fatalf("requester request status=%d result=%+v", response.StatusCode, result)
	}
	<-notifier.ch

	// 但 approver 的操作一律 401。
	checks := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodGet, "/v1/device/pending", nil},
		{http.MethodPost, "/v1/device/pair-code", nil},
		{http.MethodGet, "/v1/device/devices", nil},
		{http.MethodPost, "/v1/device/revoke", map[string]string{"id": "x"}},
		{http.MethodPost, "/v1/device/rename", map[string]string{"id": "x", "name": "y"}},
		{http.MethodPost, "/v1/device/renew", nil},
		{http.MethodPost, "/v1/device/push-token", map[string]string{"token": "t"}},
	}
	for _, check := range checks {
		response, body := doJSON(t, check.method, httpServer.URL+check.path, requester, check.body)
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s %s status=%d body=%s", check.method, check.path, response.StatusCode, body)
		}
	}

	// requester 发起的请求，requester 自己不能批；approver 可以拒。
	request = protocol.Request{Item: "KEY", Mode: protocol.ModeWrite, Purpose: "non-sealed", TTL: 10, Target: "/tmp/x"}
	resultCh := make(chan protocol.Response, 1)
	go func() {
		_, result := postRequest(t, httpServer.URL, requester, request)
		resultCh <- result
	}()
	notification := <-notifier.ch

	// approver 的待批准视图要能看到是谁在请求（设备 id + 名字）。
	response, body := doJSON(t, http.MethodGet, httpServer.URL+"/v1/device/pending", approver, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("approver pending status=%d body=%s", response.StatusCode, body)
	}
	var views []PendingView
	if err := json.Unmarshal(body, &views); err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].DeviceName != "requester-cli" || views[0].DeviceID == "" {
		t.Fatalf("pending views=%+v", views)
	}

	response, _ = doJSON(t, http.MethodPost, httpServer.URL+"/v1/decision/"+notification.RequestID, requester, protocol.Decision{Decision: "approve"})
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("requester decide status=%d", response.StatusCode)
	}
	response, _ = doJSON(t, http.MethodPost, httpServer.URL+"/v1/decision/"+notification.RequestID, approver, protocol.Decision{Decision: "deny"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("approver deny status=%d", response.StatusCode)
	}
	if result := <-resultCh; result.Status != protocol.StatusDenied {
		t.Fatalf("long-poll result=%+v", result)
	}

	// approver 的设备列表要带上角色。
	response, body = doJSON(t, http.MethodGet, httpServer.URL+"/v1/device/devices", approver, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("devices status=%d body=%s", response.StatusCode, body)
	}
	var listing struct {
		Devices []struct {
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"devices"`
	}
	if err := json.Unmarshal(body, &listing); err != nil {
		t.Fatal(err)
	}
	roles := map[string]string{}
	for _, device := range listing.Devices {
		roles[device.Name] = device.Role
	}
	if len(listing.Devices) != 2 || roles["phone"] != deviceRoleApprover || roles["requester-cli"] != deviceRoleRequester {
		t.Fatalf("devices=%+v", listing.Devices)
	}
}

func TestPairCodeIsSingleUse(t *testing.T) {
	_, httpServer := newTestServer(t, NewMapStore(nil), nil, &testAudit{}, 30*time.Second)
	approver := pairApprover(t, httpServer.URL)
	response, body := doJSON(t, http.MethodPost, httpServer.URL+"/v1/device/pair-code", approver, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("pair-code status=%d body=%s", response.StatusCode, body)
	}
	var codeResult struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(body, &codeResult); err != nil || codeResult.Code == "" {
		t.Fatalf("pair-code body=%s", body)
	}
	response, _ = doJSON(t, http.MethodPost, httpServer.URL+"/v1/pair/claim", "", map[string]string{"code": codeResult.Code})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("first claim status=%d", response.StatusCode)
	}
	response, _ = doJSON(t, http.MethodPost, httpServer.URL+"/v1/pair/claim", "", map[string]string{"code": codeResult.Code})
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("second claim status=%d", response.StatusCode)
	}
}

func TestClaimDecisionIsSingleUse(t *testing.T) {
	notifier := &testNotifier{ch: make(chan Notification, 1)}
	server, httpServer := newTestServer(t, NewMapStore(nil), notifier, &testAudit{}, 30*time.Second)
	request := protocol.Request{Item: "x", Mode: protocol.ModeWrite, Purpose: "p", TTL: 10, Target: "/tmp/x"}
	done := make(chan struct{})
	go func() {
		_, _ = postRequest(t, httpServer.URL, "pairing-test-token", request)
		close(done)
	}()
	notification := <-notifier.ch
	pending, ok := server.claimDecision(notification.RequestID, "deny", notification.DenySig, "", "")
	if !ok {
		t.Fatal("first claimDecision failed")
	}
	if _, ok := server.claimDecision(notification.RequestID, "deny", notification.DenySig, "", ""); ok {
		t.Fatal("second claimDecision succeeded")
	}
	server.finish(pending, protocol.Response{Status: protocol.StatusDenied, RequestID: notification.RequestID}, "decision", "deny")
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not finish after deny")
	}
}

func TestClaimDecisionRejectsExpiredPending(t *testing.T) {
	server, _ := newTestServer(t, NewMapStore(nil), nil, &testAudit{}, 30*time.Second)
	id := "expired-pending-request"
	pending := &pendingRequest{
		id:      id,
		expires: time.Now().UTC().Add(-time.Second),
		deny:    server.decisionSignature(id, "deny"),
		state:   pendingStateWaiting,
	}
	server.mu.Lock()
	server.pending[id] = pending
	server.mu.Unlock()
	if _, ok := server.claimDecision(id, "deny", pending.deny, "", ""); ok {
		t.Fatal("claimDecision accepted an expired pending request")
	}
}

func TestDecisionRejectsUnknownFields(t *testing.T) {
	notifier := &testNotifier{ch: make(chan Notification, 1)}
	_, httpServer := newTestServer(t, NewMapStore(nil), notifier, &testAudit{}, 30*time.Second)
	request := protocol.Request{Item: "x", Mode: protocol.ModeWrite, Purpose: "p", TTL: 10, Target: "/tmp/x"}
	done := make(chan struct{})
	go func() {
		_, _ = postRequest(t, httpServer.URL, "pairing-test-token", request)
		close(done)
	}()
	notification := <-notifier.ch

	raw := fmt.Sprintf(`{"decision":"deny","sig":%q,"extra":1}`, notification.DenySig)
	response, _ := doJSON(t, http.MethodPost, httpServer.URL+"/v1/decision/"+notification.RequestID, "", json.RawMessage(raw))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("decision with unknown field status=%d", response.StatusCode)
	}
	response, body := doJSON(t, http.MethodPost, httpServer.URL+"/v1/request", "pairing-test-token", json.RawMessage(`{"item":"x","mode":"write","purpose":"p","ttl":5,"target":"/tmp/x","bogus":1}`))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("request with unknown field status=%d body=%s", response.StatusCode, body)
	}
	var result protocol.Response
	if err := json.Unmarshal(body, &result); err != nil || result.Status != "invalid_request" {
		t.Fatalf("request with unknown field body=%s", body)
	}

	_, _ = postDecision(t, httpServer.URL, notification.RequestID, "deny", notification.DenySig)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not finish after deny")
	}
}

func TestSealedApproveRequiresV2Payload(t *testing.T) {
	notifier := &testNotifier{ch: make(chan Notification, 1)}
	_, httpServer := newTestServer(t, NewMapStore(nil), notifier, &testAudit{}, 30*time.Second)
	recipient, err := boxpayload.Generate()
	if err != nil {
		t.Fatal(err)
	}
	defer recipient.Clear()
	approver := pairApprover(t, httpServer.URL)

	request := protocol.Request{
		Item:          "KEY",
		Mode:          protocol.ModeWrite,
		Purpose:       "sealed",
		TTL:           10,
		Target:        "/tmp/x",
		SealPublicKey: recipient.PublicBase64(),
	}
	resultCh := make(chan protocol.Response, 1)
	go func() {
		_, result := postRequest(t, httpServer.URL, "pairing-test-token", request)
		resultCh <- result
	}()
	notification := <-notifier.ch

	// 设备批准不带 box 载荷 → 403。
	response, _ := doJSON(t, http.MethodPost, httpServer.URL+"/v1/decision/"+notification.RequestID, approver, protocol.Decision{Decision: "approve"})
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("approve without payload status=%d", response.StatusCode)
	}
	// 带了但不是 v2 信封 → decision 收下，长轮询拿到 failed。
	response, _ = doJSON(t, http.MethodPost, httpServer.URL+"/v1/decision/"+notification.RequestID, approver, protocol.Decision{Decision: "approve", Payload: "garbage"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("approve with garbage payload status=%d", response.StatusCode)
	}
	result := <-resultCh
	if result.Status != protocol.StatusFailed || result.Message != "phone payload required" {
		t.Fatalf("result=%+v", result)
	}
}

func TestRequestRejectsTTLAboveMax(t *testing.T) {
	_, httpServer := newTestServer(t, NewMapStore(nil), nil, &testAudit{}, 5*time.Second)
	request := protocol.Request{Item: "x", Mode: protocol.ModeWrite, Purpose: "p", TTL: 10, Target: "/tmp/x"}
	response, result := postRequest(t, httpServer.URL, "pairing-test-token", request)
	if response.StatusCode != http.StatusBadRequest || result.Status != "invalid_request" {
		t.Fatalf("ttl above max status=%d result=%+v", response.StatusCode, result)
	}
}

func TestPendingLimitReturns503(t *testing.T) {
	notifier := &testNotifier{ch: make(chan Notification, 2)}
	server, httpServer := newTestServer(t, NewMapStore(nil), notifier, &testAudit{}, 30*time.Second)
	server.maxPending = 2
	request := protocol.Request{Item: "x", Mode: protocol.ModeWrite, Purpose: "p", TTL: 5, Target: "/tmp/x"}
	done := make(chan struct{}, 2)
	for range 2 {
		go func() {
			_, _ = postRequest(t, httpServer.URL, "pairing-test-token", request)
			done <- struct{}{}
		}()
	}
	<-notifier.ch
	<-notifier.ch
	response, result := postRequest(t, httpServer.URL, "pairing-test-token", request)
	if response.StatusCode != http.StatusServiceUnavailable || result.Message != "too many pending requests" {
		t.Fatalf("over-limit status=%d result=%+v", response.StatusCode, result)
	}
	server.ClosePending()
	<-done
	<-done
}

func TestPendingViewCarriesSSHUser(t *testing.T) {
	notifier := &testNotifier{ch: make(chan Notification, 1)}
	_, httpServer := newTestServer(t, NewMapStore(nil), notifier, &testAudit{}, 30*time.Second)
	approver := pairApprover(t, httpServer.URL)
	request := protocol.Request{
		Item:          "deploy-key",
		Mode:          protocol.ModeSign,
		Purpose:       "p",
		TTL:           5,
		PublicKey:     "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFakeFixtureKeyForPendingViewTestOnly",
		SSHUser:       "deploy",
		CertTTL:       600,
		SealPublicKey: "dGVzdC1zZWFsLWtleS10ZXN0LW9ubHk",
	}
	done := make(chan struct{})
	go func() {
		_, _ = postRequest(t, httpServer.URL, "pairing-test-token", request)
		close(done)
	}()
	notification := <-notifier.ch
	response, body := doJSON(t, http.MethodGet, httpServer.URL+"/v1/device/pending", approver, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("pending status=%d body=%s", response.StatusCode, body)
	}
	var views []PendingView
	if err := json.Unmarshal(body, &views); err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].SSHUser != "deploy" || views[0].PublicKey != request.PublicKey || views[0].CertTTL != 600 {
		t.Fatalf("views=%+v", views)
	}
	_, _ = postDecision(t, httpServer.URL, notification.RequestID, "deny", notification.DenySig)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not finish after deny")
	}
}

// pairApproverVault：带 vault_id 配对 approver——设备落进该租户。
func pairApproverVault(t *testing.T, url, vaultID string) string {
	t.Helper()
	response, body := doJSON(t, http.MethodPost, url+"/v1/device/pair", "pairing-test-token", map[string]string{"name": "phone", "vault_id": vaultID})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("pair approver status=%d body=%s", response.StatusCode, body)
	}
	var result struct {
		DeviceToken string `json:"device_token"`
	}
	if err := json.Unmarshal(body, &result); err != nil || result.DeviceToken == "" {
		t.Fatalf("pair approver body=%s", body)
	}
	return result.DeviceToken
}

// 多租户隔离：请求只路由给同租户 approver；跨租户决定 403；
// 同租户新 approver 上岗自动把旧的踢下线（换机独占）。
func TestTenantIsolation(t *testing.T) {
	notifier := &testNotifier{ch: make(chan Notification, 4)}
	_, httpServer := newTestServer(t, NewMapStore(nil), notifier, &testAudit{}, 30*time.Second)

	approverA := pairApproverVault(t, httpServer.URL, "vault-a")
	approverB := pairApproverVault(t, httpServer.URL, "vault-b")
	// 不同租户共存：B 上岗不踢 A。
	if response, _ := doJSON(t, http.MethodGet, httpServer.URL+"/v1/device/pending", approverA, nil); response.StatusCode != http.StatusOK {
		t.Fatalf("approverA should still be valid")
	}
	requesterA := claimRequester(t, httpServer.URL, approverA)
	requesterB := claimRequester(t, httpServer.URL, approverB)

	// A 租户发起请求：B 的 pending 列表看不到、决定也批不了。
	done := make(chan struct{})
	go func() {
		_, _ = postRequest(t, httpServer.URL, requesterA, protocol.Request{
			Item: "tenant-a-secret", Mode: protocol.ModeWrite, Purpose: "p", TTL: 5, Target: "/tmp/x",
		})
		close(done)
	}()
	notification := <-notifier.ch

	_, bodyB := doJSON(t, http.MethodGet, httpServer.URL+"/v1/device/pending", approverB, nil)
	var viewsB []PendingView
	if err := json.Unmarshal(bodyB, &viewsB); err != nil || len(viewsB) != 0 {
		t.Fatalf("tenant B must not see tenant A pending: %s", bodyB)
	}
	_, bodyA := doJSON(t, http.MethodGet, httpServer.URL+"/v1/device/pending", approverA, nil)
	var viewsA []PendingView
	if err := json.Unmarshal(bodyA, &viewsA); err != nil || len(viewsA) != 1 {
		t.Fatalf("tenant A should see 1 pending: %s", bodyA)
	}

	response, _ := doJSON(t, http.MethodPost, httpServer.URL+"/v1/decision/"+notification.RequestID, approverB, map[string]string{"decision": "approve", "payload": "v2.x"})
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-tenant decision must be 403, got %d", response.StatusCode)
	}
	// B 租户设备列表/撤销也够不着 A 的：先拿 A 的 id。
	_, listA := doJSON(t, http.MethodGet, httpServer.URL+"/v1/device/devices", approverA, nil)
	var listedA struct {
		Devices []struct {
			ID string `json:"id"`
		} `json:"devices"`
	}
	if err := json.Unmarshal(listA, &listedA); err != nil || len(listedA.Devices) != 2 {
		t.Fatalf("tenant A device list should have approver+requester: %s", listA)
	}
	// B 的设备列表只有自己租户的两台，看不到 A 的。
	_, listB := doJSON(t, http.MethodGet, httpServer.URL+"/v1/device/devices", approverB, nil)
	var listedB struct {
		Devices []struct {
			ID string `json:"id"`
		} `json:"devices"`
	}
	if err := json.Unmarshal(listB, &listedB); err != nil || len(listedB.Devices) != 2 {
		t.Fatalf("tenant B device list should have approver+requester: %s", listB)
	}
	for _, d := range listedA.Devices {
		for _, e := range listedB.Devices {
			if d.ID == e.ID {
				t.Fatalf("cross-tenant device leaked: %s", d.ID)
			}
		}
	}

	// 改名：同租户可改，跨租户改了也无效。
	victim := listedA.Devices[0].ID
	_, renamedOK := doJSON(t, http.MethodPost, httpServer.URL+"/v1/device/rename", approverA, map[string]string{"id": victim, "name": "renamed-a"})
	var renameResult struct {
		Renamed int `json:"renamed"`
	}
	if err := json.Unmarshal(renamedOK, &renameResult); err != nil || renameResult.Renamed != 1 {
		t.Fatalf("same-tenant rename should succeed: %s", renamedOK)
	}
	_, renamedCross := doJSON(t, http.MethodPost, httpServer.URL+"/v1/device/rename", approverB, map[string]string{"id": victim, "name": "evil"})
	if err := json.Unmarshal(renamedCross, &renameResult); err != nil || renameResult.Renamed != 0 {
		t.Fatalf("cross-tenant rename must be a no-op: %s", renamedCross)
	}
	_, listA2 := doJSON(t, http.MethodGet, httpServer.URL+"/v1/device/devices", approverA, nil)
	var listedA2 struct {
		Devices []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"devices"`
	}
	if err := json.Unmarshal(listA2, &listedA2); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range listedA2.Devices {
		if d.ID == victim {
			found = true
			if d.Name != "renamed-a" {
				t.Fatalf("cross-tenant rename must not overwrite name, got %q", d.Name)
			}
		}
	}
	if !found {
		t.Fatal("renamed device missing from list")
	}

	// A 用 sig 拒绝掉这条（sig 绑定单条 request_id，不受租户过滤影响）。
	_, _ = postDecision(t, httpServer.URL, notification.RequestID, "deny", notification.DenySig)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not finish after deny")
	}

	// 同租户新 approver 上岗 → 旧 approver 被踢；requester 不受影响。
	approverA2 := pairApproverVault(t, httpServer.URL, "vault-a")
	if response, _ := doJSON(t, http.MethodGet, httpServer.URL+"/v1/device/pending", approverA, nil); response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old approver must be kicked (401), got %d", response.StatusCode)
	}
	if response, _ := doJSON(t, http.MethodGet, httpServer.URL+"/v1/device/pending", approverA2, nil); response.StatusCode != http.StatusOK {
		t.Fatalf("new approver should be valid")
	}
	if response, _ := doJSON(t, http.MethodGet, httpServer.URL+"/v1/device/pending", approverB, nil); response.StatusCode != http.StatusOK {
		t.Fatalf("tenant B approver must be unaffected by tenant A kick")
	}
	// requesterA 还活着：发请求能到 A2 的 pending（验证 requester 没被踢）。
	done2 := make(chan struct{})
	go func() {
		_, _ = postRequest(t, httpServer.URL, requesterA, protocol.Request{
			Item: "x", Mode: protocol.ModeWrite, Purpose: "p", TTL: 5, Target: "/tmp/x",
		})
		close(done2)
	}()
	notification2 := <-notifier.ch
	_, body2 := doJSON(t, http.MethodGet, httpServer.URL+"/v1/device/pending", approverA2, nil)
	var views2 []PendingView
	if err := json.Unmarshal(body2, &views2); err != nil || len(views2) != 1 {
		t.Fatalf("A2 should see the new request: %s", body2)
	}
	_ = requesterB
	_, _ = postDecision(t, httpServer.URL, notification2.RequestID, "deny", notification2.DenySig)
	select {
	case <-done2:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not finish after deny")
	}
}

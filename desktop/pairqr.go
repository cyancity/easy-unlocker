package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/cyancity/easy-unlocker/cli"
	"github.com/cyancity/easy-unlocker/internal/boxpayload"
)

// 扫码配对：桌面生成一次性 X25519 密钥对，QR 里只放 broker 地址、session、
// 公钥和机器名。手机扫到后走 /v1/pair/offer 让 broker 发一张密封给公钥的
// 设备令牌，桌面轮询 /v1/pair/offer/{session} 拿到 envelope 用私钥解开。
// 拍到 QR 也偷不走令牌——信封只对桌面私钥有意义。

type qrPairSession struct {
	key     boxpayload.KeyPair
	session string
	broker  string
}

// qrPayload 是 QR 里的 JSON；kind 固定 eu-pair，手机按它分流扫码内容。
type qrPayload struct {
	V      int    `json:"v"`
	Kind   string `json:"kind"`
	Broker string `json:"broker"`
	S      string `json:"s"`
	K      string `json:"k"`
	N      string `json:"n"`
}

// StartQRPair 生成配对 QR（data URI，前端直接 <img src>）。返回 session 供轮询。
func (a *App) StartQRPair(brokerURL string) (string, error) {
	broker := strings.TrimRight(strings.TrimSpace(brokerURL), "/")
	if broker == "" {
		return "", errors.New("先填 broker 地址")
	}
	key, err := boxpayload.Generate()
	if err != nil {
		return "", errors.New("无法创建配对密钥")
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		key.Clear()
		return "", errors.New("无法创建会话")
	}
	host, _ := os.Hostname()
	payload := qrPayload{
		V:      1,
		Kind:   "eu-pair",
		Broker: broker,
		S:      hex.EncodeToString(raw),
		K:      key.PublicBase64(),
		N:      host,
	}
	text, err := json.Marshal(payload)
	if err != nil {
		key.Clear()
		return "", errors.New("无法编码配对内容")
	}
	png, err := qrcode.Encode(string(text), qrcode.Medium, 256)
	if err != nil {
		key.Clear()
		return "", errors.New("无法生成二维码")
	}
	a.mu.Lock()
	a.qrSession = &qrPairSession{key: key, session: payload.S, broker: broker}
	a.mu.Unlock()
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}

// PollQRPair 前端每 2s 调一次：404 → 还没扫；拿到 envelope → 解出令牌落配置 → 完成。
// 返回 "waiting" | "paired" | 错误。
func (a *App) PollQRPair() (string, error) {
	a.mu.Lock()
	sess := a.qrSession
	a.mu.Unlock()
	if sess == nil {
		return "", errors.New("没有进行中的扫码配对")
	}
	req, err := http.NewRequest(http.MethodGet, sess.broker+"/v1/pair/offer/"+sess.session, nil)
	if err != nil {
		return "", errors.New("无法连接网关")
	}
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return "waiting", nil // 网关暂时不可达不致命，继续等
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "waiting", nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		return "waiting", nil
	}
	var offer struct {
		Envelope string `json:"envelope"`
	}
	if err := json.Unmarshal(body, &offer); err != nil || offer.Envelope == "" {
		return "", errors.New("网关返回了无效响应")
	}
	grant, err := boxpayload.Open(sess.key, sess.session, offer.Envelope)
	if err != nil {
		return "", errors.New("配对信封解不开——可能被抢读过，重新生成二维码")
	}
	defer func(b []byte) {
		for i := range b {
			b[i] = 0
		}
	}(grant)
	var issued struct {
		Token string `json:"device_token"`
		Role  string `json:"role"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(grant, &issued); err != nil || issued.Token == "" {
		return "", errors.New("配对信封格式不对")
	}
	cfg := config{
		BrokerURL:  sess.broker,
		Token:      issued.Token,
		Role:       issued.Role,
		DeviceName: issued.Name,
	}
	if cfg.Role == "" {
		cfg.Role = "approver"
	}
	if cfg.DeviceName == "" {
		if host, err := os.Hostname(); err == nil {
			cfg.DeviceName = host
		}
	}
	sess.key.Clear()
	a.mu.Lock()
	a.qrSession = nil
	a.mu.Unlock()
	if err := saveConfig(cfg); err != nil {
		return "", err
	}
	a.mu.Lock()
	a.cfg = cfg
	a.requester = &cli.Client{BrokerURL: cfg.BrokerURL, PairingToken: cfg.Token}
	a.mu.Unlock()
	_, _ = a.SyncVault()
	return "paired", nil
}

// CancelQRPair 放弃进行中的扫码（前端关页面/切手动配对时调）。
func (a *App) CancelQRPair() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.qrSession != nil {
		a.qrSession.key.Clear()
		a.qrSession = nil
	}
}




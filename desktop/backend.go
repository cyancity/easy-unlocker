package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// brokerClient 桌面端专用：待批准 / 决定 / vault 同步 / 设备管理。
// 复用 cli.Client 的请求长轮询不动——GUI 里发起请求走同一份 Request 逻辑。
type brokerClient struct {
	base  string
	token string
	http  *http.Client
}

func newBrokerClient(brokerURL, token string) *brokerClient {
	return &brokerClient{
		base:  strings.TrimRight(strings.TrimSpace(brokerURL), "/"),
		token: strings.TrimSpace(token),
		http:  &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *brokerClient) call(method, path string, body any, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, errors.New("无法编码请求")
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		return 0, errors.New("无法连接网关")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, errors.New("网关不可达")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return 0, errors.New("网关响应无法读取")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var fail struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &fail)
		if fail.Message == "" {
			fail.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return resp.StatusCode, errors.New(fail.Message)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp.StatusCode, errors.New("网关返回了无效响应")
		}
	}
	return resp.StatusCode, nil
}

// ---------- 待批准 ----------

type pendingView struct {
	RequestID     string `json:"request_id"`
	Item          string `json:"item"`
	Mode          string `json:"mode"`
	Purpose       string `json:"purpose"`
	TTL           int    `json:"ttl"`
	Target        string `json:"target"`
	Requester     string `json:"requester"`
	State         string `json:"state"`
	SealPublicKey string `json:"seal_public_key"`
	PublicKey     string `json:"public_key"`
	ExpiresAt     string `json:"expires_at"`
	ReceivedAt    string `json:"received_at"`
	DeviceName    string `json:"device_name"`
}

func (c *brokerClient) pending() ([]pendingView, error) {
	var list []pendingView
	_, err := c.call(http.MethodGet, "/v1/device/pending", nil, &list)
	return list, err
}

func (c *brokerClient) decide(requestID, decision, payload string) error {
	body := map[string]string{"decision": decision}
	if payload != "" {
		body["payload"] = payload
	}
	_, err := c.call(http.MethodPost, "/v1/decision/"+requestID, body, nil)
	return err
}

// ---------- vault 同步 ----------

type vaultPayload struct {
	Blob      string `json:"blob"`
	Wrap      string `json:"wrap"`
	UpdatedAt string `json:"updated_at"`
}

// vaultGet 404 = 还没有同步过；交给调用方区分「没库」和「网络错」。
func (c *brokerClient) vaultGet() (vaultPayload, bool, error) {
	var out vaultPayload
	code, err := c.call(http.MethodGet, "/v1/vault", nil, &out)
	if err != nil {
		if code == http.StatusNotFound {
			return vaultPayload{}, false, nil
		}
		return vaultPayload{}, false, err
	}
	return out, true, nil
}

func (c *brokerClient) vaultPut(blob, wrap string) error {
	_, err := c.call(http.MethodPost, "/v1/vault", map[string]string{"blob": blob, "wrap": wrap}, nil)
	return err
}

// ---------- 设备管理 ----------

type deviceMeta struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
	LastUsed  string `json:"last_used_at"`
	Current   bool   `json:"current"`
}

func (c *brokerClient) devices() ([]deviceMeta, error) {
	var out struct {
		Devices []deviceMeta `json:"devices"`
	}
	_, err := c.call(http.MethodGet, "/v1/device/devices", nil, &out)
	return out.Devices, err
}

func (c *brokerClient) revokeDevice(id string) error {
	_, err := c.call(http.MethodPost, "/v1/device/revoke", map[string]string{"id": id}, nil)
	return err
}

func (c *brokerClient) renameDevice(id, name string) error {
	_, err := c.call(http.MethodPost, "/v1/device/rename", map[string]string{"id": id, "name": name}, nil)
	return err
}

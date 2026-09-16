package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cyancity/easy-unlocker/internal/boxpayload"
	"github.com/cyancity/easy-unlocker/internal/protocol"
	"github.com/cyancity/easy-unlocker/internal/securepayload"
)

const maxResponseBody = 8 << 20

type Settings struct {
	BrokerURL    string
	PairingToken string
	Requester    string
	// NoProxy 为 true 时绕过环境代理直连 broker（--no-proxy 或 EASYGET_NO_PROXY）。
	NoProxy bool
}

func LoadSettings(configPath string) (Settings, error) {
	settings := Settings{
		BrokerURL: "http://127.0.0.1:8787",
	}
	if configPath == "" {
		configPath = os.Getenv("EASY_UNLOCKER_CONFIG")
	}
	if configPath == "" {
		if home, err := os.UserHomeDir(); err == nil {
			configPath = filepath.Join(home, ".config", "easy-unlocker", "config")
		}
	}
	if configPath != "" {
		if err := loadConfigFile(configPath, &settings); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Settings{}, errors.New("cannot read easy-unlocker config")
		}
	}
	if value := os.Getenv("EASY_UNLOCKER_BROKER_URL"); value != "" {
		settings.BrokerURL = value
	}
	if value := os.Getenv("EASY_UNLOCKER_PAIRING_TOKEN"); value != "" {
		settings.PairingToken = value
	}
	if value := os.Getenv("EASY_UNLOCKER_REQUESTER"); value != "" {
		settings.Requester = value
	}
	if settings.Requester == "" {
		settings.Requester = defaultRequester()
	}
	return settings, nil
}

func loadConfigFile(path string, settings *Settings) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return errors.New("config file is not private")
	}
	material, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(material), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return errors.New("invalid config line")
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		switch key {
		case "broker_url":
			settings.BrokerURL = value
		case "pairing_token":
			settings.PairingToken = value
		case "requester":
			settings.Requester = value
		}
	}
	return nil
}

type Client struct {
	BrokerURL    string
	PairingToken string
	HTTP         *http.Client
	// NoProxy 绕过环境变量代理直连 broker：代理的 CONNECT 隧道空闲超时会掐断长轮询。
	NoProxy bool
}

// newRequestKey 生成请求的幂等键：一次 Request 调用的重试共用同一个，
// 服务端按它找回原 pending 而不是开新单。
func newRequestKey() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// CheckBrokerURL 拒绝把设备令牌明文送出本机：非 loopback 只能 https。
// 确需 http（例如内网调试）设 EASY_UNLOCKER_ALLOW_HTTP=1。
func CheckBrokerURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err == nil && parsed.Scheme == "https" && parsed.Hostname() != "" {
		return nil
	}
	if err == nil && parsed.Scheme == "http" {
		host := parsed.Hostname()
		if host == "localhost" {
			return nil
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return nil
		}
		if os.Getenv("EASY_UNLOCKER_ALLOW_HTTP") == "1" {
			return nil
		}
	}
	return errors.New("Broker 地址必须是 https（本机 127.0.0.1/localhost 可用 http；确需明文设 EASY_UNLOCKER_ALLOW_HTTP=1）")
}

// httpClient 决定走不走代理。NoProxy 或 EASYGET_NO_PROXY=1 时直连：自托管
// broker 的长轮询会被代理 CONNECT 隧道的空闲超时掐断，能直连就别绕。
func (c Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	if c.NoProxy || os.Getenv("EASYGET_NO_PROXY") != "" {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		return &http.Client{Transport: transport}
	}
	return &http.Client{}
}

// requestRetryDelay 是断线重连的节奏：每次传输失败后等这么久再带同 key 重发。
const requestRetryDelay = 1500 * time.Millisecond

func (c Client) Request(ctx context.Context, request protocol.Request) (protocol.Response, error) {
	if strings.TrimSpace(c.BrokerURL) == "" || c.PairingToken == "" {
		return protocol.Response{}, errors.New("Broker 地址和 pairing token 都是必需的")
	}
	if err := CheckBrokerURL(c.BrokerURL); err != nil {
		return protocol.Response{}, err
	}
	// 幂等键：长轮询被代理/断网掐断后，带同 key 重发续等原请求，
	// 服务端按 pendingByKey 找回，手机上不会出现第二条。
	if strings.TrimSpace(request.RequestKey) == "" {
		key, err := newRequestKey()
		if err != nil {
			return protocol.Response{}, errors.New("无法创建请求键")
		}
		request.RequestKey = key
	}
	for {
		result, err := c.doRequest(ctx, request)
		if err == nil {
			return result, nil
		}
		// 只有传输层错误才重连：服务端明确返回的失败响应（如批准被拒）原样返回。
		// ctx 到期（TTL 用完/用户取消）就不再续了。
		select {
		case <-ctx.Done():
			return protocol.Response{}, errors.New("Broker 请求失败")
		case <-time.After(requestRetryDelay):
		}
	}
}

func (c Client) doRequest(ctx context.Context, request protocol.Request) (protocol.Response, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return protocol.Response{}, errors.New("无法编码请求")
	}
	url := strings.TrimRight(c.BrokerURL, "/") + "/v1/request"
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return protocol.Response{}, errors.New("无法连接 Broker")
	}
	httpRequest.Header.Set("Authorization", "Bearer "+c.PairingToken)
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient().Do(httpRequest)
	if err != nil {
		return protocol.Response{}, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBody))
	if err != nil {
		return protocol.Response{}, errors.New("无法读取 Broker 响应")
	}
	var result protocol.Response
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return protocol.Response{}, errors.New("Broker 返回了无效响应")
	}
	if result.Status == "" {
		return protocol.Response{}, errors.New("Broker 响应缺少状态")
	}
	return result, nil
}

func (c Client) Ping(ctx context.Context) error {
	if strings.TrimSpace(c.BrokerURL) == "" {
		return errors.New("Broker 地址未配置")
	}
	if err := CheckBrokerURL(c.BrokerURL); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.BrokerURL, "/")+"/healthz", nil)
	if err != nil {
		return errors.New("无法连接 Broker")
	}
	response, err := c.httpClient().Do(request)
	if err != nil {
		return errors.New("Broker 不可达")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode != http.StatusOK {
		return errors.New("Broker 健康检查失败")
	}
	return nil
}

// BrokerVersion 读 Broker 构建时注入的 release tag。老 Broker 没有 /v1/version（404）——
// 返回空串不算错，调用方自己决定跳不跳过检查。
func (c Client) BrokerVersion(ctx context.Context) (string, error) {
	if strings.TrimSpace(c.BrokerURL) == "" {
		return "", errors.New("Broker 地址未配置")
	}
	if err := CheckBrokerURL(c.BrokerURL); err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.BrokerURL, "/")+"/v1/version", nil)
	if err != nil {
		return "", errors.New("无法连接 Broker")
	}
	response, err := c.httpClient().Do(request)
	if err != nil {
		return "", errors.New("Broker 不可达")
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if response.StatusCode != http.StatusOK {
		return "", errors.New("Broker 版本查询失败")
	}
	var result struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", errors.New("Broker 返回了无效响应")
	}
	return strings.TrimSpace(result.Version), nil
}

func OpenPayload(response protocol.Response, pairingToken string) ([]byte, error) {
	if response.Status != protocol.StatusApproved || response.Payload == "" || response.RequestID == "" {
		return nil, errors.New("批准响应缺少凭据")
	}
	value, err := securepayload.Open(pairingToken, response.RequestID, response.Payload)
	if err != nil {
		return nil, errors.New("批准响应校验失败")
	}
	return value, nil
}

func OpenBoxPayload(response protocol.Response, key boxpayload.KeyPair) ([]byte, error) {
	if response.Status != protocol.StatusApproved || response.Payload == "" || response.RequestID == "" {
		return nil, errors.New("批准响应缺少凭据")
	}
	value, err := boxpayload.Open(key, response.RequestID, response.Payload)
	if err != nil {
		return nil, errors.New("批准响应校验失败")
	}
	return value, nil
}

func WriteSecret(path string, value []byte) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("写入路径不能为空")
	}
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, ".easy-unlocker-write-*")
	if err != nil {
		return errors.New("无法创建临时文件")
	}
	temporary := file.Name()
	removeTemporary := true
	defer func() {
		_ = file.Close()
		if removeTemporary {
			_ = os.Remove(temporary)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return errors.New("无法保护目标文件")
	}
	if _, err := file.Write(value); err != nil {
		return errors.New("无法写入目标文件")
	}
	if err := file.Sync(); err != nil {
		return errors.New("无法持久化目标文件")
	}
	if err := file.Close(); err != nil {
		return errors.New("无法关闭目标文件")
	}
	if err := os.Rename(temporary, path); err != nil {
		return errors.New("无法替换目标文件")
	}
	removeTemporary = false
	return nil
}

func NormalizeSecret(value []byte) []byte {
	if bytes.HasSuffix(value, []byte("\r\n")) {
		return value[:len(value)-2]
	}
	if bytes.HasSuffix(value, []byte("\n")) {
		return value[:len(value)-1]
	}
	return value
}

func ClearBytes(value []byte) {
	for i := range value {
		value[i] = 0
	}
}

func RequestContext(ttl int) (context.Context, context.CancelFunc) {
	if ttl < 1 {
		ttl = 1
	}
	return context.WithTimeout(context.Background(), time.Duration(ttl+15)*time.Second)
}

func defaultRequester() string {
	host, _ := os.Hostname()
	user := os.Getenv("USER")
	if user == "" {
		user = "agent"
	}
	if host == "" {
		host = "unknown-host"
	}
	return user + "@" + host
}

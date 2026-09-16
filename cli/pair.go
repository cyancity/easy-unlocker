package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultConfigPath 与 LoadSettings 的解析顺序一致：--config > 环境变量 > 用户目录。
func DefaultConfigPath(configPath string) string {
	if configPath != "" {
		return configPath
	}
	if configPath = os.Getenv("EASY_UNLOCKER_CONFIG"); configPath != "" {
		return configPath
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config", "easy-unlocker", "config")
	}
	return ""
}

// WriteConfig 原子写配置（0600）。pairing_token 在配对模型里就是「这台机器的设备令牌」。
func WriteConfig(configPath, brokerURL, token string) error {
	if strings.TrimSpace(configPath) == "" {
		return errors.New("配置路径不能为空")
	}
	if strings.TrimSpace(brokerURL) == "" || strings.TrimSpace(token) == "" {
		return errors.New("broker 地址和令牌都不能为空")
	}
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return errors.New("无法创建配置目录")
	}
	content := fmt.Sprintf("broker_url=%s\npairing_token=%s\n",
		strings.TrimRight(strings.TrimSpace(brokerURL), "/"), strings.TrimSpace(token))
	tmp, err := os.CreateTemp(dir, ".easyget-config-*")
	if err != nil {
		return errors.New("无法写临时配置")
	}
	name := tmp.Name()
	discard := func() {
		_ = tmp.Close()
		_ = os.Remove(name)
	}
	if err := tmp.Chmod(0o600); err != nil {
		discard()
		return errors.New("无法保护配置文件")
	}
	if _, err := tmp.WriteString(content); err != nil {
		discard()
		return errors.New("无法写入配置文件")
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return errors.New("无法关闭配置文件")
	}
	if err := os.Rename(name, configPath); err != nil {
		_ = os.Remove(name)
		return errors.New("无法替换配置文件")
	}
	return nil
}

// PairClaim 用一次性配对码换设备令牌。配对码由手机 App 生成（10 分钟、单次使用）。
func PairClaim(brokerURL, code, name string) (deviceToken, deviceName, expiresAt string, err error) {
	brokerURL = strings.TrimRight(strings.TrimSpace(brokerURL), "/")
	if brokerURL == "" {
		return "", "", "", errors.New("需要 --broker 指向网关")
	}
	if err := CheckBrokerURL(brokerURL); err != nil {
		return "", "", "", err
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return "", "", "", errors.New("需要 --code（手机 App 里生成的配对码）")
	}
	payload, err := json.Marshal(map[string]string{"code": code, "name": strings.TrimSpace(name)})
	if err != nil {
		return "", "", "", errors.New("无法编码请求")
	}
	req, err := http.NewRequest(http.MethodPost, brokerURL+"/v1/pair/claim", strings.NewReader(string(payload)))
	if err != nil {
		return "", "", "", errors.New("无法连接网关")
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", "", errors.New("网关请求失败")
	}
	defer resp.Body.Close()
	var result struct {
		DeviceToken string `json:"device_token"`
		Name        string `json:"name"`
		ExpiresAt   string `json:"expires_at"`
		Message     string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", "", errors.New("网关返回了无效响应")
	}
	if resp.StatusCode != http.StatusOK || result.DeviceToken == "" {
		message := result.Message
		if message == "" {
			message = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return "", "", "", fmt.Errorf("配对失败：%s", message)
	}
	return result.DeviceToken, result.Name, result.ExpiresAt, nil
}

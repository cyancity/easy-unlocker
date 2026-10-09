package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/cyancity/easy-unlocker/cli"
)

// config 是桌面端自己的配置：设备令牌 + 角色 + 网关地址。
// 与 easyGet 的 ~/.config/easy-unlocker/config 并列但分开——GUI 额外记角色与设备名。
type config struct {
	BrokerURL  string `json:"broker_url"`
	Token      string `json:"token"`
	Role       string `json:"role"`
	DeviceName string `json:"device_name"`
}

// configDir 桌面端配置目录：%APPDATA%/easy-unlocker（Windows）、
// ~/Library/Application Support/easy-unlocker（macOS）、~/.config/easy-unlocker（linux）。
func configDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", errors.New("找不到用户配置目录")
	}
	return filepath.Join(base, "easy-unlocker"), nil
}

func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "desktop.json"), nil
}

func loadConfig() (config, error) {
	path, err := configPath()
	if err != nil {
		return config{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return config{}, nil
	}
	var cfg config
	if json.Unmarshal(raw, &cfg) != nil {
		return config{}, nil
	}
	return cfg, nil
}

// saveConfig 原子写 desktop.json，并把同一令牌写进 easyGet 配置——
// 「GUI 配好后 easyGet 直接能用」是产品要求，不需要用户再跑一次 pair。
func saveConfig(cfg config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return errors.New("无法创建配置目录")
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return errors.New("无法编码配置")
	}
	tmp, err := os.CreateTemp(dir, ".desktop-*")
	if err != nil {
		return errors.New("无法写临时配置")
	}
	name := tmp.Name()
	discard := func() {
		_ = tmp.Close()
		_ = os.Remove(name)
	}
	if _, err := tmp.Write(raw); err != nil {
		discard()
		return errors.New("无法写入配置")
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return errors.New("无法写入配置")
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return errors.New("无法替换配置")
	}
	// 令牌同时写进 easyGet 配置：approver 令牌也能发请求，easyGet 直接用。
	return cli.WriteConfig(cli.DefaultConfigPath(""), cfg.BrokerURL, cfg.Token)
}

// wipeConfig 登出：清掉 GUI 配置与本地缓存的 vault 密文。不动 easyGet 配置——
// 那里面的令牌已被服务端撤销，留着也无害，但主动删干净更稳妥？留着：
// 用户可能还在用 easyGet 指别的网关。这里只清 GUI 自己那份。
func wipeConfig() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	_ = os.Remove(path)
	blob, err := vaultCachePath()
	if err == nil {
		_ = os.Remove(blob)
	}
	return nil
}

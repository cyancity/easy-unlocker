package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cyancity/easy-unlocker/cli"
)

// version 由 release.sh 用 -X main.version=<tag> 注入；源码构建是 "dev"。
var version = "dev"

// release tag 形如 v2026.09.16 或 v2026.09.16-2（同日第二发）。
var versionPattern = regexp.MustCompile(`^v(\d{4})\.(\d{2})\.(\d{2})(?:-(\d+))?$`)

// compareVersions 比 release tag：a>b→1, a==b→0, a<b→-1。任一侧不是合法 tag 返回 0
//（dev/空串都视作不可比——调用方决定怎么跳过）。
func compareVersions(a, b string) int {
	pa, pb := parseVersion(a), parseVersion(b)
	if pa == nil || pb == nil {
		return 0
	}
	for i := 0; i < len(pa); i++ {
		if pa[i] != pb[i] {
			if pa[i] > pb[i] {
				return 1
			}
			return -1
		}
	}
	return 0
}

func parseVersion(v string) []int {
	m := versionPattern.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return nil
	}
	parts := make([]int, 4)
	for i := 0; i < 4; i++ {
		if m[i+1] == "" {
			parts[i] = 0
			continue
		}
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return nil
		}
		parts[i] = n
	}
	return parts
}

func isReleaseVersion(v string) bool {
	return parseVersion(v) != nil
}

// updateNoticeInterval 控制「有新版本」提示的检查频率：每次成功后最多一天问一次。
const updateNoticeInterval = 24 * time.Hour

// maybeNotifyUpdate 在命令成功后顺带看一眼 Broker 报的版本（Broker 和 CLI 同一个
// release tag 出，所以 Broker 版本即最新 CLI 版本）。任何失败都静默——更新提示
// 不该影响取凭据主流程。
func maybeNotifyUpdate(settings cli.Settings, configPath string) {
	if !isReleaseVersion(version) {
		return
	}
	stamp := filepath.Join(filepath.Dir(configPath), ".version-check")
	if info, err := os.Stat(stamp); err == nil && time.Since(info.ModTime()) < updateNoticeInterval {
		return
	}
	latest := fetchBrokerVersion(settings)
	_ = os.WriteFile(stamp, []byte(time.Now().Format(time.RFC3339)), 0o600)
	if compareVersions(latest, version) > 0 {
		fmt.Fprintf(os.Stderr, "easyGet 有新版本 %s（当前 %s）。升级：curl -L -o /tmp/easyGet \"https://github.com/cyancity/easy-unlocker/releases/latest/download/easyGet-$(uname -s | tr 'A-Z' 'a-z')-$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')\" && install -m 755 /tmp/easyGet ~/.local/bin/easyGet\n", latest, version)
	}
}

func fetchBrokerVersion(settings cli.Settings) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	v, err := (cli.Client{BrokerURL: settings.BrokerURL, PairingToken: settings.PairingToken}).BrokerVersion(ctx)
	if err != nil {
		return ""
	}
	return v
}

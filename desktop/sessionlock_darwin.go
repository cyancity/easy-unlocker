//go:build darwin

package main

import (
	"os/exec"
	"strings"
)

// sessionLocked：macOS 没有暴露给普通进程的「已锁屏」API（通知要 objc 桥）。
// 用 IORegistry 的 IOConsoleLocked 标志轮询——社区验证可用的等价信号。
// 命令偶发失败按「未锁」处理，避免误伤正在用的库。
func sessionLocked() bool {
	out, err := exec.Command("ioreg", "-n", "Root", "-d1", "-a").Output()
	if err != nil {
		return false
	}
	s := string(out)
	idx := strings.Index(s, "<key>IOConsoleLocked</key>")
	if idx < 0 {
		return false
	}
	return strings.Contains(s[idx:idx+80], "<true")
}

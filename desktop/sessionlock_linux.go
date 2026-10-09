//go:build linux

package main

// sessionLocked：Linux 桌面环境太多（X11/Wayland、各发行版锁屏 DBus 不一），
// 没有通用可靠信号。目标平台是 Windows/macOS，这里返回未锁——
// 退出/重启仍然清库，只是没有锁屏联动。
func sessionLocked() bool { return false }

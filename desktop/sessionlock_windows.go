//go:build windows

package main

import (
	"strings"
	"syscall"
	"unsafe"
)

var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	procOpenInputDesktop    = user32.NewProc("OpenInputDesktop")
	procGetUserObjectInfoW  = user32.NewProc("GetUserObjectInformationW")
	procCloseDesktop        = user32.NewProc("CloseDesktop")
)

// sessionLocked：锁屏（含屏保安全桌面）时输入桌面切到 Winlogon，
// 解锁状态下是 Default——这是 Windows 上最可靠的轮询信号。
func sessionLocked() bool {
	hdesk, _, err := procOpenInputDesktop.Call(0, 0, 0x0001 /*GENERIC_READ*/)
	if hdesk == 0 {
		_ = err
		return true // 连输入桌面都打不开，按锁定处理（安全侧）
	}
	defer procCloseDesktop.Call(hdesk)
	var name [256]uint16
	var need uint32
	ret, _, _ := procGetUserObjectInfoW.Call(
		hdesk,
		2, // UOI_NAME
		uintptr(unsafe.Pointer(&name[0])),
		uintptr(len(name)*2),
		uintptr(unsafe.Pointer(&need)),
	)
	if ret == 0 {
		return true
	}
	return !strings.EqualFold(syscall.UTF16ToString(name[:]), "default")
}

package main

import (
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// watchSessionLock：一次解锁管一个桌面 session。轮询系统锁屏状态，
// 进入锁屏（Windows Win+L / macOS 锁定）就把库锁上——重启与退出天然清零，
// 不需要单独处理。
func (a *App) watchSessionLock() {
	was := sessionLocked()
	for range time.Tick(2 * time.Second) {
		locked := sessionLocked()
		if locked && !was {
			a.Lock()
			wailsRuntime.EventsEmit(a.ctx, "vault:locked")
		}
		was = locked
	}
}

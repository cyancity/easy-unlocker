//go:build darwin

package main

import wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

// macOS 上 getlantern/systray 必须占主线程，与 Wails 冲突——暂不提供托盘，
// hide-on-close 与后台通知照常工作。后续可换 fyne/systray 或原生包装。
func (a *App) runTray() {}

func (a *App) stopTray() {}

func (a *App) hideWindow() {
	if a.ctx == nil {
		return
	}
	a.windowHidden.Store(true)
	wailsRuntime.WindowHide(a.ctx)
}

func (a *App) showWindow() {
	if a.ctx == nil {
		return
	}
	a.windowHidden.Store(false)
	wailsRuntime.WindowShow(a.ctx)
}

//go:build windows

package main

import (
	"log"
	"os"

	"git.sr.ht/~jackmordaunt/go-toast/v2"
)

// initToast 注册 AppID（通知横幅上显示的应用名/图标）+ 点击通知 → 拉回窗口。
// SetAppData 写一次注册表即可持久化，失败不致命（横幅照样弹）。
func (a *App) initToast() {
	exe, _ := os.Executable()
	_ = toast.SetAppData(toast.AppData{
		AppID:    "easy-unlocker",
		IconPath: exe,
	})
	toast.SetActivationCallback(func(args string, data []toast.UserData) {
		a.showWindow()
	})
}

func (a *App) notifyNewRequest(item, requester, purpose string) {
	msg := item
	if requester != "" {
		msg += "\n来自 " + requester
	}
	if purpose != "" {
		msg += " · " + purpose
	}
	n := toast.Notification{
		AppID: "easy-unlocker",
		Title: "有新的取凭据请求",
		Body:  msg,
		// 点击横幅 = 拉回窗口；Short 时限自动消进通知中心，不留残影。
		ActivationArguments: "open",
	}
	if err := n.Push(); err != nil {
		log.Println("toast:", err)
	}
	// 窗口在托盘里 → 拉回前台并置顶到 pending 清空；批完自动收回。
	if a.windowHidden.Load() {
		a.autoShown.Store(true)
		a.showWindow()
	}
	a.autoPinned.Store(true)
	wailsTopMost(a.ctx, true)
}

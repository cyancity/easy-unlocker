//go:build !windows

package main

import (
	"log"

	"github.com/gen2brain/beeep"
)

func (a *App) initToast() {}

func (a *App) notifyNewRequest(item, requester, purpose string) {
	msg := item
	if requester != "" {
		msg += " · " + requester
	}
	if purpose != "" {
		msg += " · " + purpose
	}
	if err := beeep.Notify("easy-unlocker 待批准", msg, ""); err != nil {
		log.Println("notify:", err)
	}
	if a.windowHidden.Load() {
		a.autoShown.Store(true)
		a.showWindow()
	}
	a.autoPinned.Store(true)
	wailsTopMost(a.ctx, true)
}

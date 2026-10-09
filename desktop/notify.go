package main

import "time"

// watchPending 后端轮询 pending：窗口藏在托盘时也照样提醒。
// 只对「新出现」的请求提醒——seenReqs 按 request_id 去重，决定过/过期的清出集合。
// pending 归零时撤掉自动置顶；autoShown 收起来的窗口自动缩回托盘。
func (a *App) watchPending() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if a.ctx == nil {
			continue
		}
		a.mu.Lock()
		token, url := a.cfg.Token, a.cfg.BrokerURL
		a.mu.Unlock()
		if token == "" || url == "" {
			continue
		}
		list, err := newBrokerClient(url, token).pending()
		if err != nil {
			continue
		}
		a.mu.Lock()
		if a.seenReqs == nil {
			a.seenReqs = map[string]struct{}{}
		}
		now := map[string]struct{}{}
		var fresh []pendingView
		for _, p := range list {
			if p.State != "waiting" {
				continue
			}
			now[p.RequestID] = struct{}{}
			if _, ok := a.seenReqs[p.RequestID]; !ok {
				fresh = append(fresh, p)
			}
		}
		a.seenReqs = now
		a.mu.Unlock()
		for _, p := range fresh {
			a.notifyNewRequest(p.Item, p.Requester, p.Purpose)
		}
		if len(now) == 0 {
			a.settleAfterDecide()
		}
	}
}

// settleAfterDecide pending 清空后收尾：撤置顶；通知自动弹出的窗口缩回托盘，
// 用户手动打开的窗口保持原位不动。
func (a *App) settleAfterDecide() {
	if a.autoPinned.CompareAndSwap(true, false) {
		wailsTopMost(a.ctx, false)
	}
	if a.autoShown.CompareAndSwap(true, false) {
		a.hideWindow()
	}
}

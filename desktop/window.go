package main

import (
	"context"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// wailsTopMost 置顶/取消置顶——pending 到达时顶到最前，批完归零时撤下。
func wailsTopMost(ctx context.Context, on bool) {
	if ctx == nil {
		return
	}
	wailsRuntime.WindowSetAlwaysOnTop(ctx, on)
}

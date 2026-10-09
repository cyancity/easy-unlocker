//go:build windows || linux

package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"time"

	"github.com/getlantern/systray"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// tray 常驻 Windows/macOS 系统托盘。菜单：打开（拉回窗口）/ 退出（真退出）。
// getlantern/systray 在 Windows 上跑 goroutine 即可（darwin 走独立文件禁用）。
func (a *App) runTray() {
	systray.Run(func() {
		systray.SetTooltip("easy-unlocker")
		systray.SetIcon(trayIcon())
		mOpen := systray.AddMenuItem("打开", "显示主窗口")
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("退出", "退出 easy-unlocker")
		go func() {
			for range mOpen.ClickedCh {
				a.showWindow()
			}
		}()
		go func() {
			for range mQuit.ClickedCh {
				a.quitting.Store(true)
				wailsRuntime.Quit(a.ctx)
			}
		}()
	}, func() {})
}

func (a *App) stopTray() { systray.Quit() }

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
	// Windows 前台抢占规则：先 AlwaysOnTop 顶上去再撤，比裸 Show 可靠。
	// 但通知路径已把 autoPinned 置位要求持续置顶——撤之前看一眼，别抢跑。
	wailsRuntime.WindowSetAlwaysOnTop(a.ctx, true)
	time.AfterFunc(400*time.Millisecond, func() {
		if !a.autoPinned.Load() {
			wailsRuntime.WindowSetAlwaysOnTop(a.ctx, false)
		}
	})
}

// trayIcon 画一个 16x16 accent 圆角方块 → PNG → 包 ICO 头（Vista+ 支持 PNG 载荷）。
func trayIcon() []byte {
	const size = 16
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	accent := color.RGBA{R: 0x63, G: 0x66, B: 0xf1, A: 0xff}
	r := 4.0
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			dx := max(max(r-fx, fx-(size-r)), 0)
			dy := max(max(r-fy, fy-(size-r)), 0)
			if dx*dx+dy*dy <= r*r {
				img.SetRGBA(x, y, accent)
			}
		}
	}
	var pngBuf bytes.Buffer
	_ = png.Encode(&pngBuf, img)
	pngBytes := pngBuf.Bytes()
	var ico bytes.Buffer
	// ICONDIR
	binary.Write(&ico, binary.LittleEndian, uint16(0))
	binary.Write(&ico, binary.LittleEndian, uint16(1))
	binary.Write(&ico, binary.LittleEndian, uint16(1))
	// ICONDIRENTRY
	ico.WriteByte(size)
	ico.WriteByte(size)
	ico.WriteByte(0) // palette
	ico.WriteByte(0) // reserved
	binary.Write(&ico, binary.LittleEndian, uint16(1))  // planes
	binary.Write(&ico, binary.LittleEndian, uint16(32)) // bpp
	binary.Write(&ico, binary.LittleEndian, uint32(len(pngBytes)))
	binary.Write(&ico, binary.LittleEndian, uint32(22))
	ico.Write(pngBytes)
	return ico.Bytes()
}

# WIP: 桌面端体验重做（分支 `feat/desktop-revamp`）

## 目标

修复「开始菜单打不开」并全面重做客户端观感与易用性：

1. Windows 集成：安装器、开始菜单快捷方式、App Paths、卸载项、可选自启动、单实例。
2. DPI 修复：PerMonitorV2 manifest（此前高缩放屏上整个 UI 被位图拉伸、布局右下错位——
   这是旧版「UI 差」的主要根因之一，旧 exe 同样受影响）。
3. UI 重做：Mica 分层、Segoe UI Variable、批准卡片主角化（1s 倒计时 + 差量渲染 +
   智能默认）、原生 confirm 换成应用内对话框、设备改名入口（绑定面早有、UI 没暴露）。
4. 备注可达性：纯备注条目默认放备注，对齐 Android `VaultField.defaultFor`
   （此前桌面硬默认密码，纯备注条目批准报「这条没有内容可放」——easyGet 取备注失败的根因）。

## 关键改动

- `desktop/main.go`：`--version`；`SingleInstanceLock`（二次启动唤起既有窗口）；
  `windows.Options{Mica, WebviewIsTransparent, WindowIsTranslucent}`；
  版本注入 `-ldflags -X main.version`。
- `desktop/rsrc_windows_{amd64,arm64}.syso`：rsrc 生成（manifest `build/windows/app.manifest`
  PerMonitorV2 + 图标 `build/windows/app.ico`）。**必须提交二进制**，丢失即回到 DPI-unaware。
- `desktop/notify.go`：`pending:new` 事件——新请求到达时前端跳批准页。
- `desktop/app.go`：`Status` 带 `version`；空值错误文案区分「没密码只有备注」。
- 前端（`frontend/dist`，手写无构建链）：`ui.js`（toast/对话框/工具）、`app.js`（路由/状态/页）、
  `approve.js`（批准页）；lucide 图标内联 sprite（ISC）。错误横幅发布版保留（JS 崩溃必可见）。
- `scripts/install-desktop.ps1`：**UTF-8 带 BOM**（PS5.1 无 BOM 会按 ANSI 读，中文注释毁掉解析）。
- `scripts/release.sh`：desktop 双架构产物 + `-tags desktop,production`。

## 验证（2026-10-29，本机 Win11 150% DPI）

- 安装器实跑：快捷方式/App Paths/卸载项/自启动/文件全绿；从开始菜单 .lnk 启动成功，
  日志显示注入版本。
- 解锁→库页（真实数据）验收：导航/徽章/表格/搜索完整无截断，150% 下文字锐利
  （PrintWindow 全窗 + 视觉模型复核）。
- 用户真机走通：手机重置解锁密码 → 桌面同步 → 新密码解锁。
- 未做：批准卡片端到端（需要一条真请求）、暗色主题截图、macOS。

## 坑（给后来者）

1. Wails 裸 `go build` 缺 `desktop,production` 标签 → 启动弹英文 Error 框。
2. 无 manifest 时进程 DPI-unaware：GetWindowRect/PrintWindow 全部返回虚拟化坐标，
   症状是「DOM 说居中、屏幕上右下错位」，排查看 `exe 内是否含 PerMonitorV2 字样`。
3. CSS 组件 `display` 会压过 `[hidden]` UA 规则（作者样式胜 UA）——必须 `[hidden]{display:none!important}`。
4. magpie 的 GLM-5.3-Flash 长推理会烧光 max_tokens（finish_reason=length、content 空），
  请求要带 `thinking: {type: disabled}`。
5. 本机调试通道：window.onerror 横幅 + 临时 `__mark` 打点写 document.title/角标，
  验完删。

## 下一步

- 合并后打一次 release（桌面产物首次进 Release）。
- 暗色主题 + 批准卡片真机截图补充验收。
- macOS 桌面端（tray_darwin 仍是 no-op 桩）。

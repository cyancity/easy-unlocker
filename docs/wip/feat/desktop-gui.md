# WIP: 桌面 GUI + 多 approver + vault 密文同步（分支 `feat/desktop-gui`）

## 目标

- Windows/macOS 桌面端（Wails + Go）：配对、发请求、批准请求、浏览/复制 vault。
- 配对码带角色：`approver` 码配出桌面批准端（不触发换机独占）。
- vault.eu1 整库密文经 broker 同步：broker 只见 opaque blob，恢复码/密码永不出端。
- 配对成功后令牌顺手写进 easyGet 配置——GUI 配好 CLI 直接能用。

## 当前状态

编译通过：`go build` desktop ✅、`go vet` ✅、Android `assembleDebug` ✅。
未做端到端联调（留验收阶段）。

### 改动

- **协议面**（Go broker + CF worker 双实现同改）：
  - `POST /v1/device/pair-code` 接 `{role?}`，默认 `requester`；`approver` 码不踢人。
  - `POST /v1/pair/claim` 响应加 `role`。
  - 新增 `GET/POST /v1/vault`：租户级 opaque 密文存取（≤2MiB）。
    Go 版持久化 `data/vaults.json`（`--vault-file`/`EASY_UNLOCKER_VAULT_FILE`），
    worker 版落 DO storage `vault:<tenant>`。
- `cli/pair.go`：`ClaimResult` + `PairClaimFull`（`PairClaim` 保持旧签名委托）。
- `desktop/`（独立 go module，`replace ../` 复用 internal/boxpayload 与 cli）：
  - `config.go`：`%APPDATA%/easy-unlocker/desktop.json`（win）/
    `~/Library/Application Support/…`（mac）；配对后双写 easyGet 配置。
  - `vault.go`：vault.eu1 / password.wrap 的 Go 版解密
    （argon2id + AES-GCM，逐字对齐 Android Kotlin 实现）+ 本地密文缓存。
  - `backend.go`：broker 薄客户端（pending/decide/vault get-put/devices）。
  - `app.go`：Wails 绑定面（Pair/Unlock/Pending/Approve/Items/CopyItem/
    RequestValue/Devices/RevokeDevice/Unpair）。明文只活内存，Lock 清零。
  - `main.go` + `frontend/dist/*`：暗色 vanilla SPA（批准/库/取密/设备四页）。
- **Android**：配对码可选角色（设备页双按钮）；vault 保存/导入/解锁后
  自动 `POST /v1/vault` 推密文（含 password.wrap）。

### 关键坑

- `desktop/` 必须是独立 module：Wails 依赖树（x/crypto v0.53 等）不能
  污染根 module（cli/broker 仍 pin v0.33）。`internal/` 可见性按目录树
  判定，`replace ../` 下 desktop 合法引用 internal/boxpayload。
- 撤销端点只认设备 id 不认令牌：Unpair 先列设备找 `current`。
- `#items` 请求不走条目匹配，回 `{v,items:[{name,aliases}]}` 线格式。

## 验证结果

- `go build ./...`（根 module）+ `desktop/` 独立构建通过。
- `desktop.exe` 生成（8.3MB）。
- 协议三处文档已同步：PROTOCOL.md / PARITY.md。

## 下一步（验收）

- `wails build` 出正式包 + windows/macOS 双端冒烟。
- 端到端：手机生成 approver 码 → 桌面配对 → 手机推 vault → 桌面解锁 →
  easyGet 发请求 → 桌面批准。
- iOS 生成 approver 码 + vault 上传（当前缺口，PARITY 已记）。

## 相关文件

- `broker/server.go`、`broker/cmd/broker/main.go`
- `worker/src/index.ts`、`worker/src/brokerState.ts`
- `cli/pair.go`
- `desktop/`（全部新增）
- `android/.../SettingsPanes.kt`、`BrokerClient.kt`、`VaultRepository.kt`

## 追加（2025-10-11）：QR 扫码配对

- 新端点：`POST /v1/pair/offer`（approver 半拍）+ `GET /v1/pair/offer/{session}`（公开轮询）。
  双实现同步：Go 走 `internal/boxpayload` 密封，worker 走 `worker/src/box.ts`（WebCrypto
  X25519/HKDF/AES-GCM，类型定义不认 X25519 → `as any` 断言）。
- 密封内容 = grant JSON `{device_token, role, name}`（非裸 token：requester 无权
  列设备反查角色）；session 作 AAD，5min TTL、取一次即焚。
- 桌面：`desktop/pairqr.go`（go-qrcode 出 data URI，轮询 2s）+ 前端 setup 页
  按钮/QR 图/状态。配对成功照旧双写 easyGet 配置。
- Android：`ScanPane.kt`（CameraX + ML Kit barcode），设备页「扫码配对桌面」入口；
  `AppViewModel.pairOffer` 校验 `kind="eu-pair"` 且 QR 的 broker 必须等于当前网关
  （防跨服务器挂令牌）。新依赖：camera-camera2/lifecycle/view 1.4.1、mlkit
  barcode-scanning 17.3.0；manifest 加 CAMERA 权限。
- QR 只含 `{v,kind,broker,s,k,n}`，不含凭据；拍屏无风险。

## 部署状态（验收中）

- arm-free（arm.yolooo.cloud）broker 已升 `v2026.09.16-25-ge085b1e-dirty-qrpair`，
  `/v1/pair/offer` 路由上线（无参 400 = 存活）。
- 回滚：二进制备份在 `~/easy-unlocker/dist/broker.rollback-*`。

## 验证结果（追加）

- `go build` 根 module + desktop(production tag) + broker linux/arm64 ✅
- worker `tsc` ✅；Android `assembleDebug` ✅（同签名 `adb install -r` 无损覆盖）

## 追加 2（验收修复）：门禁 + 推送缺口

- **approve 静默失败** → 前端 catch 吞错；现在失败弹 toast 报原因。
- **库锁定门禁**：approver 未解锁时整应用只剩解锁页（导航/批准全部挡住）；
  requester 不受限。锁屏事件/手动 Lock 一律回门禁页。
- **推送缺口修复**：`repo.setPassword`/`clearPassword` 只写 password.wrap 不触发
  onSaved → VM 层补 pushVaultQuiet；`openVault` 解锁成功后也推一次——
  老库（功能上线前建的）此前从未同步过，这是 broker 404 的根因。
- **UI 重构**：oklch token 逐字对齐 android Theme.kt（dark/light 双套，
  prefers-color-scheme 跟随系统）；修 mixOklch 权重方向（quiet=14% 淡彩
  非 86% 实色）；按钮 nowrap 修 CJK 竖排；req-card 渐变改左侧 accent 条。

## 追加 3：托盘常驻 + 系统通知 + 自动前台

- **tray.go**（windows/linux）：getlantern/systray 常驻托盘，菜单「打开/退出」；
  图标代码生成（accent 圆角方块 → PNG → ICO 头）。tray_darwin.go 为 no-op 桩：
  systray 在 darwin 必须占主线程，与 Wails 冲突，暂不实现。
- **hide-on-close**：`OnBeforeClose` 默认拦截关窗 → 收进托盘；托盘「退出」置
  `quitting` 才真退，`OnShutdown` 停 systray。
- **通知**：Windows 走 go-toast/v2 原生 Toast（AppID 注册 + 点击回调拉回窗口 +
  Short 时长自动消）；darwin/linux fallback beeep。Windows 不允许应用主动删
  通知中心记录，横幅自动消是正确上限。
- **后台轮询**：`watchPending` 3s tick，seenReqs 按 request_id 去重只提醒新请求；
  新请求到达 → 通知 + `autoShown` 弹窗 + `autoPinned` 持续置顶；pending 清零
  → 撤置顶 + 自动弹出的窗口收回托盘（手动打开的窗口不动）。
- **竞态修复**：showWindow 的临时置顶（400ms）撤之前检查 autoPinned，避免与
  通知的持续置顶抢跑。
- 验收：托盘关窗 → CLI 请求 → 横幅 + 窗口置顶弹出 → 批准 → EASYGET_EXEC_OK +
  窗口自动收回 ✅

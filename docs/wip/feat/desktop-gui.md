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

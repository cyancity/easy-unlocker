# WIP: 设备/网关体验修复 + 多网关待批准（分支 `fix/gateway-device-ux`）

## 目标

用户在真机上陆续报的 5 个问题 + 1 个顺带查出的 Broker bug：

| # | 现象 | 根因 |
|---|---|---|
| a | 设备有效期显示 `1805233300` 这样的裸数字 | CF worker 的 `listDevices` 透传 epoch 毫秒，Go 返回 RFC3339，客户端只按 ISO 解析，兜底是 `take(10)` |
| b | 切网关后回设置页，设备数量还是旧网关的 | `switchPairing` 没清也没重载 `devices` |
| c | CLI 配对后手机显示的名字是 `device` | `runPair` 传空名，落到 Broker 的 `"device"` 兜底 |
| d | CLI 换了网关，旧网关仍留着这台机器的记录 | CLI 只能存一个网关；requester 无权列/撤设备，只能靠 App 或等 180 天 |
| e | 手机只轮询当前网关，别的网关来请求收不到 | watch 循环只打 `activePairing`；Android 无 `FirebaseMessagingService`，前台 FCM 静默丢弃 |
| f | CF 上给设备改名永远 400 | `handleRename` 的 `readJson` 白名单漏了 `name`（`readJson` 拒绝未知字段） |

范围决定（用户 2026-09-18 拍板）：d 走 App 端跨网关设备视图，不动协议；只做 Android，iOS 在 PARITY 标缺口；worker 改完直接 `wrangler deploy`。

## 当前状态

改完并已真机验收（见下）。分支已推送，等合并。

- worker：`listDevices` 先按原始 `created` 排序再 map，时间字段一律 `toISOString()`；`handleRename` 白名单补 `name`。已部署，version `54250a7c`。
- CLI：`runPair` 在无 `--name` 时用 `settings.Requester`（= `user@host`），并打印 Broker 认下的设备名。本机 `~/.local/bin/easyGet` 已换成该构建（旧的备份在 `/tmp/easyGet-backup`）。
- Android：
  - `data/DeviceTime.kt`：ISO / epoch 秒 / epoch 毫秒都能解析，兜底不再输出裸数字。
  - 设备按网关存（`UiState.devicesByPairing`），`devices` 是「当前网关」的计算属性；`onActiveChanged()` 挂在 `pair` / `switchPairing` / `markFromNotification` / `removePairing` 上。
  - `loadDevices()` 一次拉所有已配网关，单台失败只记在那台的 error 上；不走 `bg{}`（它的 catch 会 `releaseVault` + 弹全局错误）。
  - `loadDevicesIfStale()`：冷启动/回前台补一次设备表，设置页那行不必等点进设备页。
  - 非当前网关的轻量轮询（10s、`otherPolling` 防重入、陈旧结果丢弃）；网关行「N 条待批准」chip、待批准页提示卡、底部角标算总数；「切过去」= `switchPairingForPending`，切完自动展开请求。
  - 设备页按网关分组，改名/撤销带 `pairingId`，走那台网关的 client；撤销本机在任一网关上的记录会连本地配对一起清。
  - `silentRefresh` 丢弃陈旧响应（请求飞行途中切了网关）。
  - **没加 `FirebaseMessagingService`**：前台发现由轮询覆盖，后台仍靠系统通知栏 + 点击切网关。

## 验证结果

自动化：

- Go `make test` 全绿；新增 `TestDeviceListTimestampsAreRFC3339`、`TestPairNamesTheDevice`。
- Android `assembleDebug` + `testDebugUnitTest` 通过（`DeviceTimeTest` 5 例 + `SshCertTest` 6 例）。
- worker `tsc --noEmit` 通过；curl 实测 `/v1/device/devices` 返回 RFC3339、`/v1/device/rename` 从 400 变 200（用临时 approver 验完即撤销）。

真机（2026-09-18 夜，Android，App 由用户指纹解锁）：

| 用例 | 结果 |
|---|---|
| 1 设备页有效期 | ✅ 显示 `2027-03-13` / `2027-03-14` / `2027-03-17`，不再是裸数字 |
| 2 切网关后设置页设备数 | ✅ arm（5 台）↔ cf（2 台）切换后立刻跟着换，不用点进设备页 |
| 3 CF 上改名 | ✅ `device` → `<本机名>`，列表立即生效 |
| 4 设备页跨网关视图 + 撤销 | ✅ 同屏看到 arm/cf 两组；在 arm 是**非当前**网关时撤销设备成功（用一次性设备 `revoke-me` 验），cf 组不受影响。另经网关审计日志（`device=<id>` = `user@<本机名>`）确认本机残留就是那条同名记录，已撤销 |
| 5 多网关待批准 | ✅ 手机在 arm 时向 cf 发请求 → cf 行显示「1 条待批准」、待批准页出现「cf 有 1 条待批准 / 切过去」、底部角标 1 → 点「切过去」自动切到 cf 并展开请求 |
| 冷启动设备表 | ✅ `wrangler tail` 看到冷启动（未解锁）时发出 1 次 `GET /v1/device/devices` 且不重复，同时 `/v1/device/push-token` 对两台网关各注册一次 |

未验证的部分：

- 用例 5 的最后一步「指纹批准 → CLI 拿到值」没做（用户已睡，只有指纹没有密码）。请求已正常渲染（条目、用途、命令、按钮齐全），批准链路本轮未改动，且当天早些时候在 CF 上跑通过一次完整端到端（23s 落盘 0600）。
- 冷启动那一条是服务端观测（请求确实发出、且只发一次），UI 上的「N 台已配对」文案没有肉眼确认。

## 顺带发现（待用户决定，本轮没动）

- 网关的设备表里有两个非当前 approver（不是当前审批端）。按设计「同租户新 approver 上岗会踢掉旧的」，这两条属于历史残留；如果它们的令牌还在别的机器上，那台机器就能批准 arm 的请求。要不要清由用户定。
- 本机换上新构建的 `easyGet` 后**打不通 arm**（旧二进制可以）：arm 上部署的 broker 早于当前 main，新 CLI 带的字段被它的 `DisallowUnknownFields` 拒了。CLI 现在指向 CF，不受影响；哪天要回 arm 得先把 arm 上的 broker 重新部署。

## 下一步

1. 合并本分支（用户已授权 agent 自行合并）。
2. 若要，清掉 arm 上那两条残留 approver。
3. 本轮明确不做：iOS 代码改动、协议改动、`FirebaseMessagingService`、CLI 多网关配置。

## 相关文件

- `worker/src/brokerState.ts`、`worker/src/index.ts`
- `cli/cmd/easyget/main.go`、`cli/cmd/easyget/main_test.go`
- `android/.../data/DeviceTime.kt`、`AppViewModel.kt`、`ui/SettingsPanes.kt`、`ui/PendingPanes.kt`、`ui/AppScreen.kt`、`MainActivity.kt`
- `broker/server_test.go`、`docs/PARITY.md`

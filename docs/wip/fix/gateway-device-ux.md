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

改完，本地验证过；**真机验收未做**（App 需要用户指纹解锁，Android 侧只验到编译 + 单测）。

- worker：`listDevices` 先按原始 `created` 排序再 map，时间字段一律 `toISOString()`；`handleRename` 白名单补 `name`。已部署，new version `54250a7c`。
- CLI：`runPair` 在无 `--name` 时用 `settings.Requester`（= `user@host`），并打印 Broker 认下的设备名。已把本机 `~/.local/bin/easyGet` 换成本分支构建（旧的在 `/tmp/easyGet-backup`）。
- Android：
  - `data/DeviceTime.kt`：ISO / epoch 秒 / epoch 毫秒都能解析，兜底不再输出裸数字；`SettingsPanes`、`AppViewModel` 改用它。
  - 设备按网关存（`UiState.devicesByPairing`），`devices` 变成「当前网关」的计算属性 —— 切网关天然跟着换；`onActiveChanged()` 挂在 `pair` / `switchPairing` / `markFromNotification` / `removePairing` 上，负责刷新并把新当前网关从 `otherPending` 摘掉。
  - `loadDevices()` 一次拉所有已配网关，单台失败只记在那台的 error 上；不用 `bg{}`（它的 catch 会 `releaseVault` + 弹全局错误）。
  - 新增非当前网关的轻量轮询（10s 一轮、`otherPolling` 防重入、结果回主线程时丢弃已删/已切成的网关）。网关行显示「N 条待批准」、待批准页给提示卡，点「切过去」= `switchPairingForPending`，切完 `allowAutoOpen = true` 直接展开请求。底部 tab 角标 = 当前 + 其它网关之和。
  - 设备页按网关分组，改名/撤销都带 `pairingId`，走那台网关的 client；撤销本机在任一网关上的记录都会连本地配对一起清掉。
  - `silentRefresh` 加陈旧响应丢弃（请求飞行途中切了网关，旧结果不能污染新网关的 pending）。
  - **没加 `FirebaseMessagingService`**：前台发现由轮询覆盖，后台仍靠系统通知栏 + 点击切网关。

## 验证结果

已通过：

- Go：`make test` 全绿。新增 `TestDeviceListTimestampsAreRFC3339`（broker）、`TestPairNamesTheDevice`（cli，空名走 `user@host` + `--name` 覆盖）。
- Android：`assembleDebug` + `testDebugUnitTest` 通过（`DeviceTimeTest` 5 例 + `SshCertTest` 6 例）。
- worker：`tsc --noEmit` 通过；已部署；curl 实测 `/v1/device/devices` 返回 `2026-09-17T22:06:23.982Z` 这类 RFC3339，`/v1/device/rename` 从 400 变成 200 且列表里看到新名字（用临时 approver 验完即撤销）。
- CLI：新二进制 `easyGet ping` 正常。

未验证（需要手机指纹解锁）：

1. 设备页有效期显示 `2027-03-16` 而不是 `1805233300`。
2. 切网关后回设置页，设备数量立即是新网关的。
3. cf 上那条 `device` 改名成功。
4. 设备页同时看到 cf 与 arm 的设备；撤销 arm 上本机那条残留。
5. 在非当前网关发一条请求 → 出现「N 条待批准」chip / 提示卡 → 点切换 → 请求自动打开 → 指纹批准 → CLI 拿到值。

## 下一步

1. 打开 App 解锁，按上面 5 条跑真机验收。
2. 验收通过后合并 main（用户确认）。
3. 本轮明确不做：iOS 代码改动、协议改动、`FirebaseMessagingService`、CLI 多网关配置。

## 相关文件

- `worker/src/brokerState.ts`、`worker/src/index.ts`
- `cli/cmd/easyget/main.go`、`cli/cmd/easyget/main_test.go`
- `android/.../data/DeviceTime.kt`、`AppViewModel.kt`、`ui/SettingsPanes.kt`、`ui/PendingPanes.kt`、`ui/AppScreen.kt`、`MainActivity.kt`
- `broker/server_test.go`、`docs/PARITY.md`

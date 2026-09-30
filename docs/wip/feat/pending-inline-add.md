# WIP: 批准页就地新建条目（分支: feat/pending-inline-add）

## 需求与意图

请求到达但库里没有对应密钥时，在批准页直接新建条目：弹层表单（名称预填请求 item）→ 保存 → 回批准页自动选中 → 用户手动点批准。PRD 与异常矩阵见 [`docs/design/PRD-pending-inline-add.md`](../../design/PRD-pending-inline-add.md)。

关键取舍（已写入 PRD）：

- 入口收敛为「选择面板首行『＋ 新建条目』+ 空库时选择行直接变『新建』」，不在选择行再叠按钮。
- 保存后按打开表单时捕获的 requestId 绑定选中，不串卡；请求已结束则不选中、条目留库。
- 表单打开期间用既有 vaultHold 机制免锁（用户要切去别的 App 复制密钥）；关闭即释放。
- 保存≠批准，仍需手动点批准。Broker/CLI 零改动。

## 上下文与依赖

- Android：`ui/PendingPanes.kt`（PendingBody/SignBody/PickerSheet）、`ui/AppScreen.kt`（Sheet 路由）、`AppViewModel.kt`（saveItem/selectItem/vaultHold）、`data/VaultRepository.kt`（add/matchOrNull）。
- iOS：`Views/PendingView.swift`（pickRow/confirmationDialog）、`Views/ItemEditView.swift`（表单复用）、`AppState.swift`（saveItem/selectItem/vaultHold）、`Data/VaultRepository.swift`。
- 验收：Go broker 起 127.0.0.1:8787（`--pairing-token e2e-pair-token`）供 iOS UITest；`EasyUnlockerUITests` 加新用例。Android 无 UI 测试设施，靠编译 + 单测 + 人工核对矩阵。

## 当前状态

- [x] 已完成: 交互流程定稿 + PRD（docs/design/PRD-pending-inline-add.md）
- [x] 已完成: Android 实现（PickerSheet 新建入口/AddItemSheet/selectItemForRequest/addItemForPending/弹层持锁）
- [x] 已完成: iOS 实现（confirmationDialog 新建入口/空库直开/ItemEditView 复用/selectItem forRequest/弹层持锁）+ 3 条 UITest
- [x] 已完成: PARITY.md 同步
- [x] 已完成: 对抗 review——修复 iOS 面板手选仍绑 selectedPending 的串卡隐患，删除双端旧 selectItem 死代码
- [ ] 待办: 合并主线需用户验收

## 验证结果

- `go test ./...` / `go vet` / `go test -race`（协议零改动，全绿）
- Android `./gradlew :app:assembleDebug` 通过；`:app:testDebugUnitTest` 通过（UP-TO-DATE）
- iOS `xcodebuild ... build` 通过
- iOS `xcodebuild ... test`（iPhone 17 模拟器 + 本地 broker 127.0.0.1:8787）：8/8 通过，含新增 testFInlineAddApprove（未匹配→新建→自动选中→手动批准→approved+v2 payload）、testGInlineAddExpire（表单开着请求过期→条目入库不选中→expired）、testHInlineAddValidation（重名→表单留错→取消→拒绝→denied）
- Android 无 UI 测试设施：编译+单测+diff 人工核对矩阵
- Android 真机验收（25019PNF3C / Android 16，adb 驱动）：`easyGet env DEVIN_SELFTEST_X9 --exec` 发不存在条目请求 → 「没有完全匹配的条目」→ 选择 → 面板首行「＋ 新建条目」→ 表单名称已预填 → 填 dummy secret → 保存并选用 → 回卡显示「完全匹配」+ 密码字段选中 + 放出按钮激活（未自动批准）→ 手动「放出（不落盘）」→ 指纹 → 「已放行」→ 请求方收到 dummy 值，exit 0。测试条目已删。

## 下一步（接手指南）

1. 读本 wip 与 PRD。
2. `cd .worktree/pending-inline-add`；Android `cd android && ./gradlew :app:assembleDebug`；iOS `cd ios && xcodebuild -project EasyUnlocker.xcodeproj -scheme EasyUnlocker -destination 'platform=iOS Simulator,name=iPhone 17' build`。
3. iOS e2e 前需起 `dist/broker --listen 127.0.0.1:8787 --pairing-token e2e-pair-token`。
4. 等用户验收后按项目流程合并 main。

## 决策记录

- 2026-09-29 入口不做选择行内双按钮，收敛到选择面板首行 + 空库直达。
- 2026-09-29 保存后选中按 requestId 绑定而非 selectedPending，防多条 pending 串卡。
- 2026-09-29 表单期间持库锁（vaultHold），与批准网络调用同级安全面。

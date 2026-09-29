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
- [ ] 进行中: 双端实现（Android / iOS 并行）
- [ ] 待办: PARITY.md 同步；按 PRD §5 测试；对抗 review；提交推送

## 下一步（接手指南）

1. 读本 wip 与 PRD。
2. `cd .worktree/pending-inline-add`；Android `cd android && ./gradlew :app:assembleDebug`；iOS `cd ios && xcodebuild -project EasyUnlocker.xcodeproj -scheme EasyUnlocker -destination 'platform=iOS Simulator,name=iPhone 17' build`。
3. iOS e2e 前需起 `dist/broker --listen 127.0.0.1:8787 --pairing-token e2e-pair-token`。

## 决策记录

- 2026-09-29 入口不做选择行内双按钮，收敛到选择面板首行 + 空库直达。
- 2026-09-29 保存后选中按 requestId 绑定而非 selectedPending，防多条 pending 串卡。
- 2026-09-29 表单期间持库锁（vaultHold），与批准网络调用同级安全面。

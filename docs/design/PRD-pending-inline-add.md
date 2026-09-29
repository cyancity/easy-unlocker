# PRD: 批准页就地新建条目（pending inline add）

> 状态：待开发。双端（Android / iOS）同步实现，Broker / CLI 无协议变更。
> 需求来源：请求到达但库里没有对应密钥时，用户今天要离开批准页 → 条目页 → 新建 → 回批准页 → 手选 → 批准，六次跳转且请求 TTL 一直在倒计时。本功能把「新建」搬进批准页，保存后自动选中，用户只需再点一次批准。

## 1. 目标与非目标

**目标**

- 普通 `env` 请求与 `sign` 请求的批准页都能就地新建条目。
- 新建表单在弹层（Android `SheetScrim` / iOS `.sheet`）里完成，名称预填请求的 `item`。
- 保存成功 → 回到批准页 → 新条目自动选进**发起表单的那张请求卡**（按 `requestId` 绑定）→ 用户手动点批准。
- 表单打开期间 App 切后台不上锁（用户要切去密码管理器复制密钥，这是主路径）。

**非目标**

- 不改协议、不改 Broker、不改 CLI。`seal_public_key`、`decide`、pending 拉取全部照旧。
- 不做「自动批准」：保存≠放行，批准永远要用户手动点（生物识别或密码）。
- 不改别名记忆、字段选择、落盘提示等既有逻辑。

## 2. 现状回顾

- 未匹配：选择行显示「没有完全匹配的条目 / 需要你指定」→ 点开选择面板（Android `PickerSheet`、iOS `confirmationDialog`）→ 只能选已有条目。
- 空库：只显示「库里还没有条目，先去添加一条再回来」——死路，只能去条目页建完再回来，此时请求多半已超时。
- 选择面板没有「新建」入口；编辑页（Android `Screen.Edit` / iOS `ItemEditView`）与批准页完全隔离。

## 3. 交互流程

### 3.1 入口矩阵

「放出这一条 / 用这条 CA 签」选择行的行为按下表分叉；`#items` 名单请求不涉及（它本来就没有条目可选）。

| 场景 | 选择行尾按钮 | 点击后 |
|---|---|---|
| 未匹配且库为空 | 「新建」 | 直接开新建表单（跳过空的选择面板） |
| 未匹配且库非空 | 「选择」（不变） | 选择面板，**第一行固定「＋ 新建条目」**，下面才是已有条目 |
| 已匹配想换新的 | 「改」（不变） | 同上，面板里也保留「＋ 新建条目」 |
| sign 请求 | 同上三种 | 同上（面板标题为「用哪条 CA 签」，新建入口相同） |

设计取舍：新建入口收敛在「选择面板首行 + 空库直达」两处，不给选择行本身再叠第二个按钮——行内双按钮挤压已有的匹配状态显示，且面板首行的位置在所有场景（未匹配/已匹配/sign）下都一致。

### 3.2 新建表单（弹层）

- 标题：「新建条目」。sign 请求多一行说明：「这条会当 CA 用——密码或备注里要有一把 OpenSSH ed25519 私钥（可点下面生成）。」
- **名称**：预填请求里的 `item`，可改。改名后批准时走既有「记成别名」逻辑，不受影响。
- **密码 · 短值**、**备注 · 长文本（可空）**：与编辑页完全同款字段与占位符。
- 备注检测到 OpenSSH 私钥 → 显示既有 CA 提示；否则保留「生成一把 SSH CA 私钥填进备注」按钮。
- 校验复用条目保存规则：名必填、密码/备注至少一项、名称+别名全库唯一（忽略大小写）、非保留名（`#items`）。**校验失败只把错误显示在表单内，不关闭弹层。**
- 「取消」或点遮罩 → 关闭弹层，批准页状态不变，已填内容丢弃（与编辑页取消一致）。
- 弹层打开期间持住库锁（见 3.4）。

### 3.3 保存后

1. `repo.add` 写库 → 拿到新条目 id。
2. 用**打开表单时捕获的 `requestId`** 查 pending：
   - 请求仍在 → 把该条 pending 的 `selectedItemId` 设为新条目、`selectedField` 重置为该条目默认非空栏；它若是 `selectedPending` 一并更新。预填名未改时页面标「完全匹配」。toast「已添加」。
   - 请求已不在（过期/被撤/已被处理）→ **不**把新条目选给任何其它请求；条目照常留在库里；toast 仍是「已添加」（页面本身已有「已超时 / 请求已结束」信号）。
3. 批准按钮按现有 `ready` 逻辑点亮：普通请求看所选栏非空，sign 请求看所选条目含 CA 私钥。用户手动点批准 → 走既有生物识别/密码流程。
4. 关闭弹层 → 释放库锁（见 3.4）。

### 3.4 后台切换与上锁

双端都是「切后台即上锁」（Android `onStop → vm.lock()`，iOS `scenePhase .background → state.lock()`），但两端都已有 `vaultHold` 引用计数：批准网络调用和文件选择期间靠它免锁。

- 新建弹层**打开时 hold、关闭时 release**（保存/取消/点遮罩都算关闭）。期间用户切去密码管理器复制密钥，回来表单原样。
- 复用 `vaultHold` 而非新机制：语义相同（「有一件没做完的事，先别锁」）。
- 进程被杀等极端情况仍会上锁：解锁后回到待批准页，请求仍在（TTL 内），已填内容丢失——与编辑页被锁的代价一致，可接受。
- 安全面：hold 只把解锁窗口延长到表单关闭，与批准中 hold 同级；表单一关立刻恢复自动上锁。

### 3.5 异常矩阵

| 异常 | 期望表现 |
|---|---|
| 表单打开期间请求过期/被撤/被别人处理 | 保存仍写库；不选中；待批准区显示超时态或下一条请求；「已超时」toast（既有） |
| 保存时名称/别名撞车（含用户把预填名改成已有名） | 表单内红字错误，弹层不关闭 |
| 名称填成 `#items` | repo 层拒绝 → 表单内错误 |
| 名空、密码备注皆空 | 表单内错误 |
| sign 请求的新条目不含私钥 | 自动选中后批准钮保持禁用，既有「这条当不了 CA」提示出现 |
| 多条 pending | 选中只作用于打开表单时那张卡（requestId 绑定），不串卡 |
| 保存后、批准前请求过期 | 与现状一致：到期 pending 消失，已添加条目留库 |
| 保存时库已锁（极端竞态） | repo 抛错 → 表单内显示错误，弹层不关闭 |
| 弹层里再触发表单 | 弹层单实例，无叠加 |
| 请求里 `item` 超 256B 或有奇怪字符 | 直接作为名称预填值；保存校验与 `repo.add` 规则兜底 |

### 3.6 不变量

- 选择面板里已有的「选已有条目」路径完全不变。
- 新条目只是普通 vault 条目：批准后它正常出现在条目页/名单/备份里。
- `silentRefresh`/`applyPending` 的「保留用户手选」机制天然兼容：新选中的 `selectedItemId` 走同一 per-request 保留通道；即使不主动选中，名称=请求名时下一轮刷新的 `matchOrNull` 也会自动命中——主动选中只是为了即时性与改名场景。

## 4. 实现要点

### Android

- `Sheet` 新增 `Picker(requestId, prefill, forCA)`（带请求上下文替换原无参 `Picker`）与 `PendingAdd(requestId, prefill, forCA)`；`PendingPane.onChange` 改为携带 `req` 的回调。
- `PendingBody`/`SignBody`：`state.items.isEmpty()` 时行尾显示「新建」、点击直开 `Sheet.PendingAdd`；空库提示文案同步更新。
- `PickerSheet`：加标题参数（普通「放出哪一条」/ sign「用哪条 CA 签」，顺手对齐 iOS 既有差异）、首行「＋ 新建条目」（accent 样式）、`onAddNew` 回调。
- 新 `AddItemSheet` composable：复用 `Field`/`QuietButton`/`PrimaryButton`，字段与 `EditPane` 一致（含生成 CA 私钥按钮与 CA 检测提示），错误显示 `state.formError`。
- `AppViewModel`：
  - 抽出 `saveItem` 的校验为 `validateItem(editingId, name, secret, note): String?`，`saveItem` 与 `addItemForPending` 共用。
  - `addItemForPending(requestId, name, secret, note): Boolean`——校验 → `repo.add` → 更新 `items` → `selectItemForRequest` → toast；失败写 `formError` 返回 false。
  - `selectItemForRequest(requestId, itemId)`——按 requestId 定位 pending 写 `selectedItemId`/`selectedField`，是当前卡则同步 `selectedPending`；找不到请求则不动。
  - 现有 `selectItem(id)` 可改为内部委托 `selectItemForRequest(selectedPending.id, id)`。
  - 公开 `holdVault`/`releaseVault`（或加专用命名包装），供 `Sheet.PendingAdd` 打开/关闭时调用；`AppScreen` 用 `DisposableEffect` 挂接生命周期。
- `AppScreen`：`PendingPane` 的 `onPick(req)` 决定开 `Picker` 还是直开 `PendingAdd`（空库+未匹配时）；`PickerSheet` 的 `onAddNew` 转入 `Sheet.PendingAdd`；`AddItemSheet` 的 `onSave` 调 `vm.addItemForPending`，返回 true 才关弹层。
- `MainActivity`：不需要改（hold 走 `vm.lock()` 内部的 `vaultHold` 检查）。传两个新回调进 `AppScreen`。

### iOS

- `PendingView`：`@State pendingAdd: AddItemContext?`（`requestId/prefill/forCA`），`.sheet(item:)` 承载 `NavigationStack { ItemEditView(...) }`。
- `confirmationDialog`：items 列表之前加 `Button("新建条目…")`；空库时 `pickRow` 直开 sheet（跳过 dialog）。
- `ItemEditView`：加可选参数 `initialName`、`forCA`（显示 CA 说明行）、`onAdded: ((VaultItem) -> Void)?`；保存成功后若带 `onAdded`，按名称在新 `state.items` 里找回条目回调出去，再 toast + dismiss。
- `AppState`：新增 `selectItem(_ id: String, forRequest requestId: String)`（按 requestId 写选择）；`holdVault`/`releaseVault` 开放给视图层（或专用命名包装），sheet onAppear/onDisappear 挂接。
- 注意 `confirmationDialog` → `.sheet` 的转场：按钮动作里先设 context 让 dialog 收、sheet 随状态弹出；若实测 sheet 不出现，延迟一帧/几百毫秒再置 context（有 UITest 覆盖）。

### 共同

- `PARITY.md` App 功能面新增一行：批准页就地新建条目（选择面板入口 + 空库直达 + 保存自动选中）。
- 本 wip：`docs/wip/feat/pending-inline-add.md`。

## 5. 测试用例

> iOS 走 `EasyUnlockerUITests` 既有 harness（mock broker 8787 + pair-claim requester token + `approveByPassword`）。Android 无 UI 测试设施，靠单测可覆盖的纯逻辑 + 构建 + 人工冒烟核对矩阵。

| # | 用例 | 步骤 | 断言 |
|---|---|---|---|
| T1 | 未匹配→面板新建→自动选中→批准 | 发 `item=NEW_KEY` 请求 → 批准页「没有完全匹配的条目」→ 选择 → 面板「新建条目…」→ 名称已预填 → 填密码保存 | 回批准页显示 `NEW_KEY` +「完全匹配」；密码批准 → Broker `approved` + `v2.` 载荷 |
| T2 | 空库直达 | 空库下发请求 → 行尾是「新建」 | 点击直接开表单，不经选择面板 |
| T3 | 添加期间请求过期 | ttl=8s 请求 → 开表单 → 等过期 → 保存 | 条目入库（条目页可见）；无错误选中；出现超时提示 |
| T4 | 名称冲突校验 | 已有 `GH_TOKEN` → 表单把名改成 `GH_TOKEN` 保存 | 表单内错误、弹层不关；改成唯一名可保存 |
| T5 | 取消不落地 | 开表单填一半 → 取消 | 无新条目；批准页回到未匹配态 |
| T6 | sign 请求新建 CA | sign 请求（item=新 CA 名）→ 新建 → 生成私钥填备注 → 保存 → 批准 | 自动选中、批准钮亮；Broker `approved` |
| T7 | 选中不串卡 | 两条 pending，对第一条开新建 | 选中只落在第一条，第二条不受影响（单测/代码审查覆盖，e2e 可选） |
| T8 | 改名后批准记别名 | 预填名改成别的保存 → 批准 | 既有别名记忆仍生效（`rememberAlias` 开时 req.item 记为别名） |
| T9 | 切后台不丢表单 | 表单填一半 → 切后台 → 回前台 | 表单与已填内容保留（hold 生效） |

## 6. 验收口径

- 双端均实现 3.1 入口矩阵与 3.3 保存后行为；异常矩阵逐条核对。
- `go test ./...`（回归）、Android `:app:assembleDebug` + `:app:testDebugUnitTest`、iOS `xcodebuild build` + `EasyUnlockerUITests`（至少跑 T1/T3 新用例与既有 A2 批准回归）。
- `docs/PARITY.md` 与 `docs/wip/feat/pending-inline-add.md` 同步。

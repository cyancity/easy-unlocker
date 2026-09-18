# WIP: 命令块 ❯ 基线对齐（分支 `fix/command-baseline`）

## 目标

用户真机反馈：批准页 / 记录详情里命令块（深色终端框）的 `❯` 没有跟第一行文字水平对齐。

根因：`CommandBlock` 的 `Row` 没设垂直对齐，默认顶对齐；命令折行时 `❯` 贴在第一行的字顶，看起来偏高。iOS 侧用的是 `HStack(alignment: .firstTextBaseline)`，两端不一致。

## 当前状态

已改完、真机截图确认。

- `ui/Components.kt` 的 `CommandBlock`：给 `❯` 与命令文字都加 `Modifier.alignByBaseline()`（`RowScope` 成员，不需要 import）。语义等价 iOS 的 `.firstTextBaseline`。
- iOS 不动（本来就是对的）。

## 验证结果

- `assembleDebug` 通过，已 adb 安装到真机。
- 真机对比（记录详情，命令折成两行）：改前 `❯` 明显高于第一行；改后 `❯` 坐在第一行基线上。UI 树坐标上命令文字整体下移 14px 去对齐 `❯` 的基线。

## 下一步

等用户确认观感后合并。

## 相关文件

- `android/app/src/main/java/io/github/cyancity/easyunlocker/ui/Components.kt`

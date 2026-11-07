# easy-unlocker — Agent 指引

编码助手入口：[`docs/AGENTS-SETUP.md`](docs/AGENTS-SETUP.md)（安装、配置、构建、发布全流程）。

要点速记：

- 禁止把 pairing token、恢复码、密钥写进对话/commit/日志；取凭据走 `easyGet`，不落盘。
- 不在 main 上改代码；新功能开 `.worktree/<slug>` 分支。
- 双端规则：功能/协议改动必须同步 [`docs/PARITY.md`](docs/PARITY.md)（双端需求树）。
- Broker 双实现（Go + CF worker）必须同改，见 `docs/PLAN.md` 坑16。
- 分支开发维护 `docs/wip/` 交接文档。
- 跑安卓模拟器用 [`scripts/emulator.sh`](scripts/emulator.sh)：必须 `-gpu host`（SwiftShader 是 CPU 软渲染，会把宿主 CPU 打满）；起模拟器前先跟用户说一声，用户机器上还有别的活。

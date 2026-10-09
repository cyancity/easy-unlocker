# WIP: easyGet Windows 原生支持（分支 `fix/windows-native`）

## 目标

修复 easyGet 在 Windows 原生环境（无 WSL）下的安装与运行失败。

## 当前状态

已改完，Windows 本机 `go test ./...` 全绿，`go vet ./...` 干净，
linux/amd64、darwin/arm64、windows 三平台 `go build` 通过。

### 改动

- `cli/configperm_unix.go` / `configperm_windows.go`：配置权限检查拆平台。
  - Windows 下 unix 位检查无意义（Go 一律报 0666），跳过；
    `hardenPrivateFile` 用 icacls 关继承、只授当前 SID 完全控制
    （grantee 用 SID `*S-1-5-…`，不用用户名）。
- `cli/exec_unix.go` / `exec_windows.go`：`--exec` 拆平台。
  - unix：`sh -c` + `cmd.ExtraFiles` 匿名 fd（`/dev/fd/3`），不变。
  - Windows：密钥写 %TEMP% 下私有临时文件（ACL 收紧到当前用户），
    `EASYGET_SECRET_FD` 传文件路径，子进程退出后删除。
  - 命令不直接 `cmd /C <command>`——cmd 对 /C 整串做引号剥离，
    命令带引号即解析错乱；改写进临时 `.cmd` 批处理执行，语义等价。
- `cli/cmd/easyget/main.go`：help 文案区分 unix `/dev/fd/3` 与
  Windows 临时文件两种形态；`USER`/`USERNAME` 兜底取用户名。
- `cli/pair.go`、`cli/client.go`：Windows 路径/权限相关小改。
- `scripts/install.ps1`：Windows 原生安装脚本。下载 release 到
  `%LOCALAPPDATA%\Programs\easy-unlocker`，写用户 PATH，
  可选 `-Broker -Code` 直接 pair，最后 `easyGet ping` 自检。
- 测试适配：unix-only 断言（0600 mode 位、HOME 隔离）在 Windows 跳过；
  新增 `cli/exec_windows_test.go` 验证环境变量 + fd 文件双载体。

### 关键坑

- `cmd /C` 引号规则：Go exec 给含空格参数加引号 → cmd 剥首引号后对
  多引号串处理不可靠，重定向语法报错。解法：`.cmd` 批处理文件。
- Windows 临时文件不能用 unix 的 "写完立刻 unlink 留 fd" 手法
  （ExtraFiles 不支持），只能留名+用后删，文档里如实说明差异。

## 验证结果

- `go test ./...`：全过（Windows，含 `--exec` env+文件双载体用例）。
- `go vet ./...`：干净。
- 三平台交叉编译通过。

## 下一步

- `install.ps1` 在干净环境走一遍真实安装（下载→PATH→pair→ping）。
- 与手机端做一次 Windows 原生端到端验收（在最终验收阶段统一做）。

## 相关文件

- `cli/exec.go`、`cli/exec_unix.go`、`cli/exec_windows.go`、`cli/exec_windows_test.go`
- `cli/configperm_unix.go`、`cli/configperm_windows.go`
- `cli/client.go`、`cli/pair.go`、`cli/cmd/easyget/main.go`
- `scripts/install.ps1`

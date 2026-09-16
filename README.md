<p align="center">
  <img src="assets/logo/easy-unlocker-mark.svg" alt="easy-unlocker" width="140">
</p>

<h1 align="center">easy-unlocker</h1>

<p align="center">
  <b>把手机变成 Agent 的审批台。</b><br>
  Agent 要密钥，不用贴进对话。你手机上按个指纹，密钥直接送到那个进程手里。
</p>

<p align="center">
  <a href="README.en.md"><b>English</b></a> · <b>中文</b>
</p>

<p align="center">
  <a href="https://github.com/cyancity/easy-unlocker/releases/latest"><img src="https://img.shields.io/github/v/release/cyancity/easy-unlocker" alt="release"></a>
  <img src="https://img.shields.io/badge/license-Apache--2.0%20%2F%20AGPL--3.0-blue" alt="license">
  <img src="https://img.shields.io/badge/platform-Android%20·%20iOS%20·%20CLI-lightgrey" alt="platforms">
</p>

<p align="center">
  <a href="#装和用">装和用</a> ·
  <a href="docs/AGENTS-SETUP.md">给 Agent 的说明书</a> ·
  <a href="docs/PROTOCOL.md">协议规范</a> ·
  <a href="SECURITY.md">安全模型</a>
</p>

---

## 为什么做这个

AI Agent 干活总会卡在同一个地方：它要 API Key，要 SSH 私钥，要一个你并不想给它看的密码。

最顺手的处理是贴进对话框。快是快，这条明文从此就躺在模型上下文、聊天记录和各种日志里。讲究一点的做法是打开密码管理器，复制，粘贴，再确认没粘错窗口。十次里有九次，人会选贴对话框——懒是真实的，安全流程拗不过它。

easy-unlocker 的做法是把安全这条路修得比粘贴还省事：

```bash
easyGet env OPENAI_API_KEY --exec 'python run.py' --for "调 openai 接口"
# → 手机弹出：谁(user@laptop) · 要什么(OPENAI_API_KEY) · 拿去干嘛
# → 指纹一按，值直接进 python 进程的环境变量，不落盘
```

每次 `easyGet` 现场生成一把一次性钥匙，公钥随请求送到手机，手机用它把值封成密文，只有你这次命令手里的私钥能开。**中转服务器从头到尾只见密文。**

```
┌────────┐  ① 请求(带一次性公钥)   ┌────────┐  ② 推送/轮询   ┌────────┐
│  CLI   │ ────────────────────▶ │ Broker │ ─────────────▶ │  手机  │
│ easyGet│ ◀──── ④ 密封值 ─────── │ 转发   │ ◀──── ③ 批准+封 │ 指纹   │
└────────┘                       └────────┘                 └────────┘
   只有 ① 的私钥能开 ④                只见密文                   明文唯一出口
```

> 它不是密码管理器，不替代 Bitwarden，不做浏览器自动填充。只存你愿意放给 Agent 的那几条。

## 装和用

这套东西由三件组成：

| 组件 | 干什么 |
|---|---|
| 手机 App（`android/` `ios/`） | 存条目、指纹或密码批准、在本机签 SSH 登录证书 |
| `easyGet` 命令行（`cli/`） | Agent 调用的入口：发请求、收密封值、写文件或注入子进程 |
| Broker 中转（`broker/` `worker/`） | 配对、按你的库路由请求、转发密文。要自己跑一台：默认 Cloudflare Worker（一条 `wrangler deploy`），也可以在 VPS 上跑 Go 二进制 |

**1. 装 easyGet**（macOS / Linux / Windows 都有编译好的包）：

```bash
os=$(uname -s | tr 'A-Z' 'a-z'); arch=$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')
curl -L -o /tmp/easyGet "https://github.com/cyancity/easy-unlocker/releases/latest/download/easyGet-${os}-${arch}"
install -m 755 /tmp/easyGet ~/.local/bin/easyGet
```

Windows 在 [Releases](https://github.com/cyancity/easy-unlocker/releases/latest) 里下 `easyGet-windows-amd64.exe`。

**2. 装手机 App**：Android 直接装 Releases 里的 `easy-unlocker-debug.apk`。iOS 源码在 `ios/`，用 Xcode 打开自签安装。装好后创建保险库（恢复码自动存到本机文件），在设置里配上 Broker。

**3. 配对**：App → 设置 → 设备 → 生成配对码（8 位，10 分钟有效），然后电脑上跑：

```bash
easyGet pair --broker https://broker.example.com --code <8位码>
easyGet ping   # 通了，顺便告诉你有没有新版
```

> **提示**：Android 端收不到推送时，排查通知权限是否开启、电量策略设为「无限制」、允许自启动——不少 ROM 默认限制后台。

**用起来**就三种请求：

```bash
# 最常见：值只进子进程环境变量，不落盘
easyGet env OPENAI_API_KEY --exec 'python run.py' --for "调 openai 接口"

# 要文件就明说（0600）
easyGet env OCI_KEY --write-to ~/.oci/key.pem --for "oci cli 读 key_file"

# 不知道库里有什么名字？先查名单（免费，不惊动手机）
easyGet list

# SSH 登录：手机签一张几分钟有效的证书，CA 私钥不出手机
easyGet ssh my-ssh-ca --for "登录服务器跑诊断"
```

> **你是 Agent / 编码助手？** 安装、配对、三种模式怎么选、哪些事别做，都写在 [docs/AGENTS-SETUP.md](docs/AGENTS-SETUP.md)。照着做，别让人把密钥贴给你。可选：把 [`skills/easy-unlocker/`](skills/easy-unlocker/SKILL.md) 装进你的 skills 目录。

## 功能与进度

### 已经能用

- 手机审批放行：每条请求看清「谁、要什么、拿去干嘛、放到哪」，指纹或密码批准，也可以拒绝
- 两种交出方式：写文件（权限 0600），或直接注入子进程环境变量、用完即焚
- 条目名单：Agent 先 `easyGet list` 查名字再开口，不用瞎猜着反复弹你手机
- SSH 短时证书：手机拿库里的 CA 私钥签几分钟有效的登录证书，私钥永远不出手机
- 新机器自助配对：配对码 10 分钟一次性，不用人工搬令牌；已配设备能列表、撤销、续期、改名
- 保险库之间互相隔离：请求只路由到你这个库的设备；换新手机自动把旧机踢下线
- 多网关：同时配几台 Broker 随时切换，点通知自动跳到发请求的那台
- Bitwarden 备份导入、密文备份导出/导入（换机迁移）、恢复码；批准记录只存本机
- 推送：Android 走 FCM，锁屏也能收到；App 开着时自动轮询
- `easyGet` 自己会比版本，有新版会提醒你

### 各端进度

| 端 | 状态 |
|---|---|
| Android | 功能全量，日常真机在用 |
| iOS | 与 Android 基本对齐，真机测过；缺 .eu1 密文导入，APNs 推送未接入 |
| easyGet | macOS / Linux / Windows 三平台发版 |
| Broker | Go 与 Cloudflare Worker 两份实现等价，线上都在跑 |
| 介绍页 | 在独立仓库维护（`site/` 已拆出本仓） |

### 接下来

- iOS 收推送：APNs 需要付费开发者账号，目前 iOS 靠 App 打开时轮询兜底
- 按字段放出：一条备注里只挑出 Agent 要的那几项给它
- CLI 回写：Agent 产出的新凭据写回手机库
- 多人共用一台 Broker 的配额与邀请机制；更远一步是托管订阅

## 信任模型一句话

> **你不用信 Broker。** 它能看到条目名、用途、设备名这些元信息；条目值、CA 私钥全程端到端密封，它解不开。完整威胁模型见 [SECURITY.md](SECURITY.md)，协议逐字段规范见 [docs/PROTOCOL.md](docs/PROTOCOL.md)。

## 文档

| 文档 | 内容 |
|---|---|
| [docs/AGENTS-SETUP.md](docs/AGENTS-SETUP.md) | 给编码助手的安装/构建/发布全流程 |
| [docs/PROTOCOL.md](docs/PROTOCOL.md) | HTTP 协议 + 密封格式 + 租户模型 |
| [SECURITY.md](SECURITY.md) | 漏洞报告渠道 + 威胁模型 |
| [docs/PARITY.md](docs/PARITY.md) | Android/iOS/Broker/CLI 双端需求树 |
| [docs/DEPLOY.md](docs/DEPLOY.md) | 自建 Broker 部署 |

## 许可

| 目录 | 协议 |
|---|---|
| `cli/` `android/` `ios/` `internal/` `docs/` 等 | [Apache-2.0](LICENSE) |
| `broker/` `worker/` | [AGPL-3.0](broker/LICENSE)：自建自由；改一改对外提供托管服务，修改必须开源 |

# easy-unlocker — MVP 计划

> **历史文档（M1 时期）**：这一版 MVP 的批准端是自托管推送服务的通知按钮，该方案已于 2026-09-14 弃用并全部移除。
> 现在批准端是手机 App（指纹 + 手选条目 + 本地密封），当前实现见 [P1-APP.md](P1-APP.md)。本文保留作当时的设计记录。
>
> 目标：**让作者本人在只有一台手机的情况下，能救回被鉴权 block 的 Agent。**
> 全局设计见 [PLAN.md](PLAN.md)。本文件只描述跑通这一件事所需的最小范围。

---

## 1. MVP 的边界

### 做

- 单用户，无账号体系，用预共享 token
- Broker 单实例
- 存储后端只接 **Bitwarden**（作者已用，零迁移）
- 两类凭据：**SSH 登录**、**API Key 写入**
- 手机批准走**现成推送通道**，不做 App
- 审计日志落文件

### 明确不做（留给后续阶段）

- 手机 App（P1）
- 账号注册、多用户、计费（P3）
- 内置保险库 / 非 Bitwarden 后端（P2）
- 无人在场的自动化
- 高可用、备份、恢复码
- 界面（除手机上的批准动作外）

---

## 2. 部署形态（MVP 阶段）

| 组件 | 放哪 | 说明 |
|---|---|---|
| **Broker** | 作者的 Mac | `rbw` 已经就绪，不需要迁移保险库 |
| **easyGet CLI** | Agent 所在机器（如 `your-server`） | 单文件，一行命令安装 |
| **批准端** | 手机浏览器 / ntfy App | 不做原生 App |
| **推送** | 自托管 ntfy | action button 直接 POST 回 Broker，不需要写 bot |

**已知代价**：Broker 在 Mac 上，所以 **Mac 必须保持唤醒**（`caffeinate` 或电源设置里禁止休眠）。
这是 MVP 阶段的有意识取舍——换取"今天就能跑起来"。P1/P2 把存储后端抽象出来后，Broker 可迁到常开的服务器。

**为什么用 ntfy 而不是复用现有 Discord bot**：ntfy 的 action button 是"按钮 → HTTP POST"开箱即用，零 bot 代码；Discord 按钮需要写 interaction 处理。MVP 阶段选阻力最小的。
**必须自托管 ntfy**——用 ntfy.sh 会把工具名、命令、路径送到第三方。

---

## 3. 协议（M0 定义，后续不改）

### CLI → Broker

```
POST /v1/request
Authorization: Bearer <pairing-token>        # 只有请求权限
{
  "item":    "ssh-key-your-server",          # 要什么
  "mode":    "sign" | "write",               # 怎么用
  "purpose": "登录服务器维护",                # 为什么（会显示在手机上）
  "ttl":     300,                            # 用多久（秒）
  "target":  "/opt/app/.env",                # mode=write 时的落地路径
  "requester": "agent@your-server"           # 谁在要
}
```

Broker 挂起连接（long-poll，最长 `ttl`），直到用户决定或超时。

M3 的 `sign` 请求可额外携带 `public_key`（OpenSSH 公钥）与 `ssh_user`；这两个字段不改变基础请求语义，也不允许携带私钥。响应会带 `request_id` 供 CLI 校验 payload 绑定。

### Broker → CLI

```
200 { "status": "approved", "mode": "sign", "payload": "<短时证书或加密值>" }
403 { "status": "denied" }
408 { "status": "expired" }                  # 超时一律当拒绝
```

**`payload` 对 CLI 之外的任何进程不可见**，尤其不打印到 stdout——`easyGet` 的输出只有状态和一句人话。

### 批准回调

ntfy action button 携带**单次签名 token**（HMAC，绑定 request id，用后即废）POST 到：

```
POST /v1/decision/<request-id>   { "decision": "approve" | "deny", "sig": "..." }
```

实现为批准和拒绝按钮分别生成一个只使用一次的 HMAC；每个 sig 同时绑定 request id 和对应的 decision，防止把“拒绝”请求体替换成“批准”。

---

## 4. 里程碑

每个里程碑都必须**独立可验证**。上一个没通过就不要进下一个。

### M0 — 骨架与协议（约 0.5 天）

- Broker 起 HTTP 服务，实现 `/v1/request`，能把请求存进内存待批准队列
- 请求带 pairing token 校验；无 token / 错 token 返回 401
- 待批准队列有超时清理

**验收**：
```bash
curl -s -X POST localhost:8787/v1/request -H "Authorization: Bearer $TOKEN" \
  -d '{"item":"test","mode":"write","purpose":"冒烟","ttl":60}'
# → 挂起；另一端可见待批准项；60s 后返回 408
```
另需验证：不带 token → 401；两个并发请求互不干扰。

---

### M1 — 推送与批准（约 0.5 天）

- 待批准请求推送到手机（自托管 ntfy）
- 手机上显示：**谁在要 / 要什么 / 为什么 / 用多久**
- 批准 / 拒绝按钮回传，Broker 收到决定

**验收**：跑 M0 的 curl，手机在 2 秒内收到通知；点「批准」后 curl 立刻返回 `approved`，点「拒绝」返回 `denied`。**不点，60 秒后返回 `expired`。**
另需验证：伪造 `sig` 的批准请求被拒；同一个 sig 重放第二次被拒。

---

### M2 — API Key 端到端（约 1 天）

- Broker 用 `rbw get` 取条目
- 值经加密通道回传 CLI
- CLI 写入 `--write-to` 指定的路径，写完从内存清除
- `easyGet` 的 stdout 只有 `已完成`，**不含值**

**验收**：
```bash
ssh your-server 'easyGet env OPENAI_API_KEY --write-to /tmp/t.env --for "冒烟"'
# 手机批准后：
ssh your-server 'grep -c sk- /tmp/t.env'      # → 1（值确实写进去了）
ssh your-server 'cat /tmp/t.env | wc -c'      # 内容正确
```
**关键验证**：`easyGet` 的输出、Broker 日志、CLI 日志里**都搜不到密钥明文**。
```bash
grep -r "$(rbw get OPENAI_API_KEY)" ~/.easy-unlocker/logs/ /tmp/easyGet.log ; echo "应为空"
```

---

### M3 — SSH 短时证书（约 1 天）

- **CA 私钥只存手机保险库**（条目备注栏放 OpenSSH ed25519 私钥）；Broker 不持有 CA
- `easyGet ssh <ca-item> --for "..."` → 手机指纹批准 → 手机本地签一张短时证书，密封回传
- 目标机 `sshd_config` 加 `TrustedUserCAKeys`；CLI 证书 + 临时身份落 `~/.ssh/`，`ssh -i` 直接登录
- `--ssh-user`（默认本机用户名）与 `--cert-ttl`（默认 300s，上限 86400）可选

**验收**：
```bash
ssh your-server 'easyGet ssh my-ssh-ca --for "冒烟"'
# 手机批准 → 证书落到 ~/.ssh/ 并完成一次登录
# 证书过期后新连接被拒（已建立的会话不受影响——证书只在登录握手时校验）：
ssh -i ~/.ssh/easy-unlocker-my-ssh-ca your-server 'echo x'   # 过期后 → Permission denied
```

**私钥不传输**：CA 私钥只在手机，Broker 只见公钥与密文；用户原有 SSH 私钥不分发。
`akr`（Akamai）是这条路线的成熟先例，可参考其交互设计。

**Plan B**：若 CA 配置在目标机上遇到阻碍，退化为"临时 ssh-agent 注入"——`easyGet` 拿到私钥后灌进一个只存活本次命令的 ssh-agent，用完销毁。**代价：私钥会到达目标机内存，违反 PLAN.md 3.1 的原则，属于明确的技术债，须在 P1 还清。**

---

### M4 — 硬化与交接（约 0.5 天）

- 配对 token 的作用域收敛到"仅请求"
- 所有失败路径 fail closed；超时=拒绝
- 审计日志：时间 / 谁 / 要什么 / 决定 / 结果（**不含任何值**）
- 一键安装脚本（`easyGet` CLI 装到目标机 + 写 `AGENTS.md` 指令 + `easyGet ping` 自检）
- 写 `docs/wip/` 交接文档

**验收**：断网 / 关手机 / Broker 挂掉三种情况下，Agent 都拿到明确的失败而不是挂死；审计日志能回答"上周谁批准了什么"。
安装脚本在**一台干净的新服务器**上从零跑通，全程手动步骤为零（除了手机上点一次批准）。

---

## 5. 风险与对应的止损点

| 风险 | 症状 | 止损 |
|---|---|---|
| SSH CA 在目标机配置受阻 | M3 卡超过 1 天 | 切 Plan B（临时 ssh-agent），记为技术债 |
| Mac 休眠导致 Broker 不可达 | 请求超时 | `caffeinate` 顶着；若频繁发生，提前把 Broker 迁到常开服务器 |
| ntfy 自托管不稳定 | 通知丢失 | 通知失败必须让请求快速超时，而不是静默悬挂 |
| 范围蔓延 | 开始做 App / 账号 / 界面 | 回看第 1 节"明确不做" |

---

## 6. MVP 完成的定义

在**只有手机**的情况下，完成一次真实操作：

> Agent 在 `your-server` 上被鉴权挡住 → 手机上收到一句话 → 点批准 → Agent 把事办完。
> 全程没有复制粘贴明文，密钥没有进入对话、日志、模型上下文。

跑通这一次，MVP 即成立，可以进入 P1。

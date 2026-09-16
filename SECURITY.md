# 安全政策

## 报告漏洞

**不要**在公开 issue 里报告安全漏洞。

请通过 GitHub 私有漏洞报告私下报告：仓库页面 → Security → Report a vulnerability

请在报告中说明：受影响组件（cli / broker / worker / android / ios）、复现路径、影响评估。预期 72 小时内收到确认回复。

## 威胁模型

### 设计目标

敏感值（API key、私钥、密码、SSH CA 私钥）**永不进入**：Agent 上下文、对话记录、shell 历史、日志、Broker 存储与内存。

- 手机是唯一的批准平面和明文出口。
- Broker 只转发请求元数据与密封载荷——**broker 不可信也能保住机密性**（见下）。
- SSH CA 私钥只在手机本地，签发短寿命 OpenSSH 用户证书。

### 信任边界

| 组件 | 能看到 | 不能看到 |
|---|---|---|
| Broker | 条目名、用途、目标路径、请求方标识、设备名、租户 UUID、时间戳、FCM token | 条目值、CA 私钥、证书明文（v2 密封）、库内容 |
| Agent / CLI 宿主 | 批准后的值（按设计交付到子进程或文件） | 未批准的任何内容、库其余条目 |
| 手机 | 一切（它是明文唯一出口） | — |
| 配对码/令牌持有者 | 其角色允许的操作面 | 其他租户的任何存在性 |

### 恶意 Broker 能做什么

拖延、丢弃、伪造 pending（但没有一次性决策签名，伪造的批准回不到请求方）；向 requester 塞假 payload（X25519 密封 + request_id 绑定，伪造过不了 AEAD）。**机密性不依赖 broker 诚实**；可用性依赖（broker 宕机=服务不可用）。

### 已知限制

- v1 securepayload（无 seal_public_key 的兼容路径）的机密性依赖 pairing token 不泄露——新请求一律 v2。
- pairing token 是「开租户」能力：持 token 者可用任意 vault_id 上岗并踢掉该租户原 approver（DoS 面）——Phase 2 计划 per-user 邀请码收敛。
- FCM 推送通道把 request_id + 决策 sig 交给 Google 基础设施——sig 是一次性的，泄露窗口即请求 TTL。

## 支持的版本

仅 main 分支最新 release 受支持。安全修复直接进 main 并随下一个 release 发布。

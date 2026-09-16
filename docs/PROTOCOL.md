# easy-unlocker 协议规范 v1

> Broker 的 HTTP 协议与载荷格式。Go broker（`broker/`）与 CF worker（`worker/`）是同一协议的两份实现——本文件是两者的共同契约，改动必须同步。
>
> **信任前提**：Broker 只转发密文，永不接触明文。它能看到的元数据见文末「Broker 视野」。

## 1. 角色与令牌

| 角色 | 凭据 | 能力 |
|---|---|---|
| 全局 pairing token | `Authorization: Bearer <token>` | 服务器管理员持有；配对 approver 上岗 + 直发请求（归入 `default` 租户） |
| approver 设备令牌 | `Authorization: Bearer <token>` | 拉 pending、做决定、配对码、设备管理、推送注册 |
| requester 设备令牌 | `Authorization: Bearer <token>` | 发起请求 |
| admin token | `X-Admin-Token: <token>` | 跨租户 pending 视图（运维） |
| 决策签名 | body `sig` | 一次性 HMAC，绑定单个 request_id + 决定方向（approve/deny 各一枚）。FCM 推送载荷携带，通知直达也能批 |

所有设备令牌 180 天有效，可随时被同租户 approver 撤销。

## 2. 租户模型

- **vault_id = 租户**：App 建库/首次解锁时本地生成 UUID，存进加密区（导出/导入沿用 = 换机同租户）。
- approver 配对时声明 `vault_id` 绑定租户；**同租户新 approver 上岗自动撤销旧 approver**（换机独占语义）。
- 配对码绑生成者的租户；requester claim 后继承同租户。
- **requester 不携带租户字段**——Broker 从其令牌反查，伪造不了。
- pending / decision / FCM / 设备列表 / 撤销 / 改名全部按租户过滤；老设备记录（无 tenant 字段）归 `default`。
- admin token 视野跨租户（P2 开源后视情况收敛）。

## 3. 端点

### 公共

| 端点 | 说明 |
|---|---|
| `GET /healthz` | `{status: "ok"}` |
| `GET /v1/version` | `{version: "vYYYY.MM.DD[-n]"}` — release tag，CLI 更新检查用 |

### 请求方（requester）

| 端点 | 认证 | 说明 |
|---|---|---|
| `POST /v1/request` | requester 令牌 或 pairing token | **长轮询**：挂到批准/拒绝/超时/断开。body = `Request`，响应 = `Response` |
| `GET /v1/device/pending` | — | requester 不能用（401，approver 专属） |

### 批准方（approver）

| 端点 | 认证 | 说明 |
|---|---|---|
| `GET /v1/device/pending` | approver | 本租户 pending 列表（PendingView[]） |
| `POST /v1/decision/{id}` | approver **或** body `sig` | `{decision: "approve"\|"deny", sig?, payload?}` → `{status:"accepted"}`。sig 与令牌任一即可；approve 密封请求必须带 `payload` |
| `POST /v1/device/pair` | pairing token | `{name, vault_id?}` → `{device_token, name}`。approver 上岗入口 |
| `POST /v1/device/pair-code` | approver | → `{code, expires_at}`（10 分钟，一次性） |
| `GET /v1/device/devices` | approver | → `{devices: [{id,name,role,created_at,expires_at,last_used_at?,current}]}` |
| `POST /v1/device/revoke` | approver | `{id}` → `{status:"ok",removed}`。撤销自己 = 登出 |
| `POST /v1/device/rename` | approver | `{id, name}` → `{status:"ok",renamed}` |
| `POST /v1/device/renew` | approver（本机） | `{}` → `{status:"ok",expires_at}`，再延 180 天 |
| `POST /v1/device/push-token` | approver | `{token}` → `{status:"ok"}`，注册 FCM token |

### 设备配对（无凭据入口）

| 端点 | 认证 | 说明 |
|---|---|---|
| `POST /v1/pair/claim` | — | `{code, name}` → `{device_token, name, expires_at}`。requester 上岗入口 |

### 运维

| 端点 | 认证 | 说明 |
|---|---|---|
| `GET /v1/admin/pending` | `X-Admin-Token` | 全租户 pending 视图 |
| `POST /v1/admin/forget-device` | `X-Admin-Token` | （worker 版）删除指定设备 |

## 4. 线格式

### `Request`（POST /v1/request）

```json
{
  "item":            "条目名（必填，≤256B）",
  "mode":            "write | sign（必填）",
  "purpose":         "用途说明，批准页显示（必填，≤4KB）",
  "ttl":             60,
  "target":          "write 模式的目标路径提示（≤4KB）",
  "requester":       "user@host（≤512B）",
  "delivery":        "file | ephemeral（默认 file；仅 write 有意义）",
  "seal_public_key": "一次性 X25519 公钥，rawURL base64",
  "public_key":      "（sign 必填）authorized_keys 行格式的 SSH 公钥",
  "ssh_user":        "（sign）证书 principal",
  "cert_ttl":        "（sign）证书有效期秒，≤86400；0 = 签发方默认",
  "request_key":     "客户端幂等键（≤128B，可空）"
}
```

校验要点：`item`/`purpose` 必填；`ttl` 1 ≤ x ≤ broker `--max-ttl`（默认 1h）；`mode=sign` 强制 `public_key` + `seal_public_key`；`item="#items"` 是 list 握手的保留名。

#### 幂等续等（request_key）

长轮询可能被代理 CONNECT 超时、断网、Wi-Fi 切换掐断。客户端给请求一个随机 `request_key` 后：

- Broker 按 (request_key, tenant) 索引 pending；同租户带同 key 重发 → **续等原请求**（`request_reattached`），不重推 FCM，手机上不出现第二条
- 客户端连接断开 → pending 标 `detached` 进 **15s 重连宽限期**（`request_detached`）；宽限期内未续才真取消（`request_cancelled`），手机待批准随之消失
- 老客户端（无 request_key）行为不变：断开即取消。新客户端打老 Broker：重发会开新单重推——兼容但不优雅
- CLI 行为：`Client.Request` 自动生成 key 并在传输错误时 1.5s 间隔重发直到 TTL；`--no-proxy`/`EASYGET_NO_PROXY` 可绕开环境代理直连

### `Response`（长轮询返回）

```json
{ "status": "approved | denied | expired | failed",
  "mode": "write|sign",
  "payload": "v2.<b64 密文>",
  "request_id": "…",
  "message": "失败原因（可选）" }
```

HTTP 码：approved→200，denied→403，expired→408，failed→503。

### `PendingView`（GET /v1/device/pending 元素）

Request 全字段 + `request_id`、`received_at`、`expires_at`、`state`（waiting/deciding/…）、`device_id`/`device_name`（发起方设备）。**payload 不在其中**——批准时才由手机产出。

### 决策签名

`sig = HMAC-SHA256(decisionKey, "easy-unlocker/v1/decision/" + request_id + "/" + decision)`。approve/deny 各一枚，单次使用，claim 后该请求进 `deciding` 状态不再接受第二次。FCM 推送载荷携带两枚 sig——通知里的直达批准路径走的就是它。

## 5. 密封载荷（应用层加密）

### `v2.` boxpayload（现役）

```
key   = HKDF-SHA256( X25519(eph_priv, seal_pub), info = request_id )
ct    = AES-256-GCM(key, nonce12, plaintext, aad = request_id)
wire  = "v2." + b64url( eph_pub[32] | nonce[12] | ct )
```

手机用请求里的 `seal_public_key` 封；只有持有对应一次性私钥的 CLI 进程能开。request_id 进 HKDF 和 AAD 双重绑定，换 request 重放失败。

### `v1.` securepayload（兼容路径）

`key = HKDF(pairing_token, request_id)` → AES-GCM。老 CLI 不传 `seal_public_key` 时走这条——此时**服务端 mock store 或手机本地都可能产出**（视部署形态），信任面比 v2 宽。新请求一律 v2。

## 6. sign 模式（SSH 证书）

`mode=sign` 时手机把选中条目当 **OpenSSH CA 私钥**用：

1. CLI 生成 ed25519 一次性身份 + `seal_public_key`，请求带 `public_key`/`ssh_user`/`cert_ttl`
2. 手机批准页展示：登录用户、证书有效期、公钥指纹、选哪条 CA
3. 批准后手机本地签出 OpenSSH user cert（principals=[ssh_user]，5 个 `permit-*` 扩展，生效时间回拨 120s 抗时钟偏差），证书本体走 boxpayload 密封回去
4. **CA 私钥永不离开手机**；Broker 和 CLI 只见证书

## 7. 推送与轮询

- approver 注册 `push-token` 后，新请求触发 FCM：`{request_id, approve_sig, deny_sig}`，**只发同租户设备**
- 无 FCM 时 App 前台 3s 轮询 `GET /v1/device/pending` 兜底

## 8. Broker 视野（威胁模型）

**看得到**：item 名、purpose、target 路径、requester 字符串、mode、设备名/id、租户 UUID、时间戳、推送 token。
**看不到**：任何条目值、CA 私钥、证书明文（v2 密封后）、vault 内容。
**能作恶**：拖延/丢弃请求、发假推送（但没有有效 sig 就批不了东西；伪 approver 也只能批到假 pending）。机密性不依赖 broker 诚实。

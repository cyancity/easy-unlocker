# easy-unlocker 本地运行与验收

本文是本地跑 Broker / CLI 的操作备忘。批准端是手机 App（见 [P1-APP.md](P1-APP.md) 与 [android/README.md](../android/README.md)）；推送只有 FCM。生产部署时请让 Broker 通过 HTTPS 暴露（例如放在 Caddy 后面，或给 Broker 同时配置 `--tls-cert` 与 `--tls-key`），因为 pairing token 仍然需要保密。

## 构建与测试

```bash
go build -trimpath -o dist/broker ./broker/cmd/broker
go build -trimpath -o dist/easyGet ./cli/cmd/easyget
go test ./...
go vet ./...
go test -race ./...
```

产物是 `dist/broker` 与 `dist/easyGet`。`logs/`、`data/` 和本地凭据路径已被 `.gitignore` 忽略。

## M0：只验证协议和超时

```bash
TOKEN='只在本机 shell 中保存的随机 token'
./dist/broker --listen 127.0.0.1:8787 \
  --pairing-token "$TOKEN" \
  --audit-log logs/broker.jsonl
```

另开终端执行。这个请求会保持挂起，直到 60 秒超时：

```bash
curl -sS -X POST localhost:8787/v1/request \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"item":"test","mode":"write","purpose":"冒烟","ttl":60}'
# → {"status":"expired", ...}，HTTP 408
```

无 token 应返回 HTTP 401：

```bash
curl -sS -o /dev/null -w '%{http_code}\n' \
  -X POST localhost:8787/v1/request \
  -H 'Content-Type: application/json' \
  -d '{"item":"test","mode":"write","purpose":"冒烟","ttl":60}'
```

如启动时配置 `--admin-token`，可用下面的只读接口查看待批准元数据。pairing token 不可用于该接口：

```bash
curl -sS localhost:8787/v1/admin/pending \
  -H "X-Admin-Token: Bearer $ADMIN_TOKEN"
```

## M1：本机直连批准

本机验证时不需要任何推送通道：直接把 Broker 起在 `127.0.0.1:8787`，用设备令牌走 `GET /v1/device/pending` + `POST /v1/decision/<id>` 就能批（手机 App 走的就是这条）。

```bash
./dist/broker \
  --listen 127.0.0.1:8787 \
  --pairing-token "$TOKEN" \
  --admin-token "$ADMIN_TOKEN" \
  --audit-log logs/broker.jsonl
```

手机会看到谁、要什么、用途和 TTL；批准或拒绝后 pending 立刻消失。伪造和重放 decision 都返回 403。通知（FCM）发布失败会让挂起请求快速以 `failed` 结束，不会静默等待。

## M2：API Key 写入

生产默认使用 `rbw`，Broker 只在用户批准后执行 `rbw get <item>`。本机没有 `rbw` 时可以用显式的 mock store 验证协议，不要把真实密钥放进仓库：

```bash
EASY_UNLOCKER_MOCK_ITEM=OPENAI_API_KEY \
EASY_UNLOCKER_MOCK_SECRET='仅用于本地测试的值' \
./dist/broker --store mock --pairing-token "$TOKEN" \
  --audit-log logs/broker.jsonl
```

真实使用：

```bash
EASY_UNLOCKER_BROKER_URL='https://unlock.example' \
EASY_UNLOCKER_PAIRING_TOKEN="$TOKEN" \
./dist/easyGet env OPENAI_API_KEY \
  --write-to /tmp/service.env \
  --for '修复服务登录'
```

批准后 `easyGet` 只输出 `已完成`，把值写到目标文件并清理内存；Broker 响应中的 payload 是按 pairing token 和 request id 绑定的 AES-GCM envelope，不是明文。值不会进入 easyGet stdout、Broker 审计日志或错误文本。写入文件权限为 `0600`，目标父目录必须已存在。拒绝、过期和发放失败都会返回非零退出码，阻止 Agent 继续执行。

## M3：SSH 短时证书

CA 私钥只存在手机保险库里（Broker 不持有 CA）。先在 App 里建一条 CA 条目：新建条目 →「生成一把 SSH CA 私钥填进备注」→ 保存。条目详情页会展示 **SSH CA 公钥**（可复制）——把这行公钥放到目标机的受信位置，在 `sshd_config` 中配置：

```text
TrustedUserCAKeys /etc/ssh/easy-unlocker-ca.pub
```

`sshd -t` 验证后 `reload`（不踢现有连接）。之后请求证书：

```bash
EASY_UNLOCKER_BROKER_URL='https://unlock.example' \
EASY_UNLOCKER_PAIRING_TOKEN="$TOKEN" \
./dist/easyGet ssh my-ssh-ca \
  --ssh-user ubuntu \
  --cert-ttl 300 \
  --for '临时登录修复服务'
```

`my-ssh-ca` 是那条 CA 条目的名字（对不上手机上手选）。批准后证书写 `~/.ssh/easy-unlocker-my-ssh-ca-cert.pub`、临时身份写 `~/.ssh/easy-unlocker-my-ssh-ca`（0600），`ssh -i` 会自动加载同目录的 `-cert.pub`。也可 `--identity ~/.ssh/id_ed25519` 复用现有身份（仍只发公钥出去）。

证书默认 300 秒有效（`--cert-ttl` 上限 86400）；**只在 SSH 握手时校验**，已建立的会话不受过期影响。目标机还须允许该 principal。签名全程在手机本地完成，Broker 只见请求元数据、公钥和密封后的证书密文。

## M4：安装与审计

在目标机已能访问 Broker 的前提下，安装脚本可以从当前源码构建，也可以下载一个已发布的 easyGet 二进制：

```bash
EASY_UNLOCKER_BROKER_URL='https://unlock.example' \
EASY_UNLOCKER_PAIRING_TOKEN="$TOKEN" \
EASYGET_BINARY="$PWD/dist/easyGet" \
./scripts/install.sh
```

脚本把 `easyGet` 安装到 `~/.local/bin/easyGet`，把配置写到权限为 `0600` 的 `~/.config/easy-unlocker/config`，向当前目录的 `AGENTS.md` 添加最小调用规则，并运行 `easyGet ping`。无 Go 环境的干净服务器可改用 `EASYGET_URL`。

审计日志是 JSONL，只包含时间、request id、item、mode、requester、target、决定和结果；不包含 purpose、签名、pairing token 或任何凭据值。超时、拒绝、通知失败、存储失败均 fail closed。

## 当前机器的验证边界

当前开发环境已验证 Go 单元/集成测试、M0 HTTP 401/408、并发请求、HMAC 防伪防重放、加密 payload、CLI 写入权限、审计无明文、手机端签发的证书通过 `ssh-keygen -L` 对拍（格式、principal、有效期、签名可验）。真机 FCM 通知到达、真机指纹批准和目标 sshd 登录需要按上面步骤验收。

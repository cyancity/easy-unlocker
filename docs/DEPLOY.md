# 自建 Broker（网关）

Broker 是手机和 CLI 之间的中转：配对、按库路由请求、转发密文。它只见密文，解不开条目值。

两份实现，协议逐字兼容，App 和 CLI 不用改，任选其一：

| 方式 | 目录 | 适合 |
|---|---|---|
| Cloudflare Worker（**默认**） | `worker/` | 不维护服务器；免费额度够单人用；一条 `wrangler deploy` 上线 |
| Go 二进制 + VPS | `broker/` | 已有公网机器、想完全自控 |

## 方式一：Cloudflare Worker（默认）

需要一个 Cloudflare 账号（免费版即可）。

```bash
cd worker
npm install
npx wrangler login      # 浏览器授权一次
```

注入三个密钥（值自己生成，别贴进对话）：

```bash
npx wrangler secret put EASY_UNLOCKER_PAIRING_TOKEN   # 手机第一次配对用，openssl rand -hex 24
npx wrangler secret put EASY_UNLOCKER_ADMIN_TOKEN     # 管理端点用，openssl rand -hex 16
npx wrangler secret put EASY_UNLOCKER_DECISION_KEY    # 决策签名，openssl rand -hex 32
```

要锁屏推送（Android FCM）再注入服务账号 JSON：

```bash
# 这份 JSON 从 Firebase 控制台「服务账号 → 生成新私钥」拿，见 FCM.md
npx wrangler secret put EASY_UNLOCKER_FCM_CREDENTIALS < fcm-service-account.json
```

部署：

```bash
npx wrangler deploy
# → https://easy-unlocker-broker.<你的子域>.workers.dev
```

`wrangler.toml` 里 `name` 就是子域前半段，可改。`[vars]` 的 `CLI_VERSION` 填最近一次 release tag，`/v1/version` 靠它告诉 CLI 有没有新版；发版后改成新 tag 再 deploy 一次，不改则 CLI 跳过更新检查（不影响功能）。

想用自有域名也可以在 CF 控制台绑 custom domain，`EASY_UNLOCKER_PUBLIC_URL` 语义不变（见下）。

## 方式二：Go 二进制 + VPS

适合已有一台公网机器的人。需要：VPS、一个指向它的域名、反代出 HTTPS。

**1. 拿二进制**：Release 里有 `broker-linux-arm64`；或在服务器上用 Go 容器编译：

```bash
git clone https://github.com/cyancity/easy-unlocker.git ~/easy-unlocker && cd ~/easy-unlocker
docker run --rm -v "$PWD":/src -w /src golang:1.24 \
  bash -c 'go build -trimpath -buildvcs=false -o dist/broker ./broker/cmd/broker'
```

**2. 写 `.env`**（`chmod 600`，不入库不打印）：

```bash
umask 077
cat > .env <<EOF
EASY_UNLOCKER_PAIRING_TOKEN=$(openssl rand -hex 24)
EASY_UNLOCKER_ADMIN_TOKEN=$(openssl rand -hex 16)
EASY_UNLOCKER_MOCK_ITEM=test
EASY_UNLOCKER_MOCK_SECRET=local-mock-only
EASY_UNLOCKER_STORE=mock
EASY_UNLOCKER_LISTEN=127.0.0.1:8787
EASY_UNLOCKER_AUDIT_LOG=$HOME/easy-unlocker/logs/broker.jsonl
EASY_UNLOCKER_DEVICE_FILE=$HOME/easy-unlocker/data/devices.json
EASY_UNLOCKER_FCM_CREDENTIALS=$HOME/easy-unlocker/data/fcm.json
EASY_UNLOCKER_PUBLIC_URL=https://broker.example.com
EOF
chmod 600 .env && mkdir -p logs data
```

`EASY_UNLOCKER_PUBLIC_URL` 填你的公网地址——它写进推送载荷当「网关标识」，App 靠它知道请求来自哪台网关（worker 版自动取请求 origin，不用配）。

**3. systemd 常驻**：

```ini
[Unit]
Description=easy-unlocker broker
After=network-online.target

[Service]
EnvironmentFile=/home/<you>/easy-unlocker/.env
ExecStart=/home/<you>/easy-unlocker/dist/broker
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

**4. 反代**，Caddy 例子：

```caddy
broker.example.com {
  encode gzip
  handle /v1/* {
    reverse_proxy 127.0.0.1:8787
  }
  handle /healthz {
    reverse_proxy 127.0.0.1:8787
  }
  handle {
    respond 404
  }
}
```

先配 DNS 再改反代，否则空烧 ACME。

## 接入与验证

```bash
curl -s https://broker.example.com/healthz     # 200
curl -s https://broker.example.com/v1/version  # 报版本串
```

手机 App：设置 → 配对，Broker 填你的网关地址，pairing token 用上面生成的那个（只在自己终端取，别贴进对话）。之后新机器不用碰 token：App → 设置 → 设备 → 生成配对码，CLI 跑 `easyGet pair --broker <地址> --code <8位码>` 自助上机。

冒烟：

```bash
easyGet env test --write-to /tmp/eu-test.env --for "部署冒烟" --ttl 120
# 手机收到通知 → 指纹 → 批准 → 终端「已完成」
```

审计日志：Go 版在 `logs/broker.jsonl`；worker 版进 Workers Logs（`npx wrangler tail`）。两边都不应出现明文值或 token。

## 不要做的事

- 不要把 pairing token、admin token、FCM 服务账号写进仓库、聊天或日志。
- 不要在服务端放密码管理器的解锁态。数据面在手机，服务端 `--store mock` 就够。

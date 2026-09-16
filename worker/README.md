# easy-unlocker Broker on Cloudflare Workers

把 Broker 的协议与状态机用 TypeScript 重写在 Workers + Durable Object 上，与 Go 版逐字兼容，**App 与 CLI 一行不改**，只换 base URL。这是默认推荐的网关形态：不用维护公网服务器，免费额度够单人用。

- 部署自己的一份：照 [`docs/DEPLOY.md`](../docs/DEPLOY.md) 「方式一」走（`wrangler login` → secrets → `wrangler deploy`）。

## 目录

```
worker/
├── wrangler.toml          # DO binding + migrations
├── src/index.ts           # 路由、鉴权、/v1/request 建单+推送+等决策
├── src/brokerState.ts     # Durable Object：pending 记录、设备表、TTL alarm
├── src/fcm.ts             # service account → WebCrypto RS256 JWT → FCM v1
├── src/sign.ts            # HMAC 决策签名（对齐 Go 的 decisionSignature）
└── tools/fakephone/       # 假手机（Go，用仓库同一份 boxpayload 密封）
```

## 本地跑（不需要 CF 账号）

```bash
cd worker
npm install
# .dev.vars 已在 .gitignore 里；三个值随便生成，spike 用
{ echo "EASY_UNLOCKER_PAIRING_TOKEN=$(openssl rand -hex 16)";
  echo "EASY_UNLOCKER_ADMIN_TOKEN=$(openssl rand -hex 16)";
  echo "EASY_UNLOCKER_DECISION_KEY=$(openssl rand -hex 32)"; } > .dev.vars
npx wrangler dev --port 8787 --ip 127.0.0.1     # 8787 正好是 easyGet 的默认地址
```

### 用假手机走一遍

```bash
cd ..                                           # 仓库根
go build -o /tmp/easyGet-spike ./cli/cmd/easyGet
PAIRING=$(grep '^EASY_UNLOCKER_PAIRING_TOKEN=' worker/.dev.vars | cut -d= -f2)

# ⚠️ easyGet env 不吃 --broker/--token（见「坑」一节），必须用环境变量或配置文件
export EASY_UNLOCKER_BROKER_URL=http://127.0.0.1:8787
export EASY_UNLOCKER_PAIRING_TOKEN="$PAIRING"

DEVICE=$(go run ./worker/tools/fakephone -pair -pairing "$PAIRING")

/tmp/easyGet-spike env SPIKE_KEY --write-to /tmp/spike.env --for "冒烟" --ttl 60 &
go run ./worker/tools/fakephone -token "$DEVICE" -item SPIKE_KEY -secret sk-spike-value
wait                                            # 期望：stdout「已完成」，/tmp/spike.env 内容 = sk-spike-value，权限 0600
```

假手机支持 `-deny`（拒绝）、`-item`（挑哪条请求）、`-http` 之外的默认地址 `-broker`。

## Phase A 实测结果（2026-09-14，本地 workerd）

| 用例 | 结果 |
|---|---|
| 正常批准：CLI 建单 → 假手机密封 → CLI 写文件 | ✅ 退出码 0，`/tmp/spike.env` 内容正确、权限 `0600`，stdout 无明文 |
| 拒绝 | ✅ easyGet 打印「已拒绝」，退出码 1，不写文件 |
| 无人批准，等 TTL 过期（ttl=20） | ✅ 恰好 20s 后「已过期」，没被提前掐断 |
| 伪造 decision（错设备令牌） | ✅ 403 `invalid_decision`，easyGet 继续等 |
| 设备 approve 但不带 `v2.` 信封 | ✅ 403（对齐 Go 的 claimDecision 规则） |
| 单次批准的开销 | **约 3 个 subrequest**（create + wait + fcm），长轮询**不轮询**，一次挂住等决策 |

关键设计：长轮询不是「每 2s 拉一次 DO」（免费版 50 subrequests/请求 撑不住 300s TTL），而是
**Worker 挂一个 DO 调用，决策到达时由 DO 唤醒**。`/v1/request 200 OK (2138ms)` 这条日志就是它：
耗时 = 真实等待时间，subrequest 数恒为常数。

## 部署到 workers.dev

```bash
cd worker
npx wrangler login                              # 浏览器授权（唯一需要你动手的一步）
npx wrangler secret put EASY_UNLOCKER_PAIRING_TOKEN   # 建议用新值，别复用生产的
npx wrangler secret put EASY_UNLOCKER_ADMIN_TOKEN
npx wrangler secret put EASY_UNLOCKER_DECISION_KEY
# FCM service account：管道直进 CF，明文不落本地、不进对话
ssh your-server 'cat ~/easy-unlocker/data/fcm.json' | npx wrangler secret put EASY_UNLOCKER_FCM_CREDENTIALS
npx wrangler deploy
```

部署后：App 的「设置 → 配对」把 Broker 填成 `https://easy-unlocker-broker.<你的子域>.workers.dev`，配对 token 用上面那个新值；
CLI 侧把 `broker_url` / `pairing_token` 换成同一组（配置文件或 `EASY_UNLOCKER_*` 环境变量）。
本机把部署用的三个值写在 **`worker/.prod.vars`**（已 gitignore，`chmod 600`），要填进 App 时 `cat` 那个文件自己复制。

⚠️ **App 里重新配对会覆盖当前的生产配对**（broker URL + 设备令牌一起换掉）。测完想切回生产：配对页填回
`https://broker.example.com` + 生产的 pairing token（在服务器 `~/easy-unlocker/.env`，或你自己的 Bitwarden）。

**回滚**：App 的 Broker 填回 `https://broker.example.com`、CLI 配置改回来即可；worker 侧 `npx wrangler delete` 或 `npx wrangler rollback`。

### Phase B 实测结果（2026-09-14，真实 CF）

| 用例 | 结果 |
|---|---|
| 部署 + healthz | ✅ `https://easy-unlocker-broker.<your-subdomain>.workers.dev`，healthz 200 / 0.85s |
| 配对 → 建单 → 假手机密封 → CLI 写文件 | ✅ 退出码 0，文件内容正确、权限 `0600`，全程 5s |
| 无人批准 ttl=40：CF 能否挂住长请求 | ✅ 恰好 40s 后过期 —— **边缘没有提前掐断**（本地只验到 20s） |
| DO `Alarm`（TTL 清理） | ✅ tail 里能看到 Alarm 触发 |
| FCM：service account → WebCrypto 签 RS256 JWT → 换 Google token | ✅ 拿到 access token（探针用假 device token，Google 回 400 `FCM send returned 400`） |
| 真机 FCM 锁屏推送 + 指纹批准 | ✅ 通知 → 指纹 → 手选 `my-api-key` → 批准 → CLI「已完成」，目标文件 0600、内容正确（31s） |

排障用到的接口：`GET /v1/admin/pending`、`POST /v1/admin/forget-device`（`{"all":true}` 清空设备表 / `{"device_token":"…"}` 解绑一台），
都要 `X-Admin-Token: Bearer <EASY_UNLOCKER_ADMIN_TOKEN>`。清测试设备就靠它。

### 剩下的风险

- 真机那条路（FCM 到 Android + App 指纹）没验过；一旦通了，spike 的结论就可以定。
- 免费版 50 subrequests/请求的余量：实测一次批准约 3 个，离上限很远；真正的约束是 DO 时长（一次批准 ≈ 等待秒数 × 128MB，免费 13,000 GB-s/天）。

## 坑

1. **`easyGet env --broker/--token` 被静默忽略**：`cli/cmd/easyGet/main.go:83` 把 `settings`（配置文件/环境变量）传给 `executeWrite`，
   而 `executeWrite` 用的是 `settings.BrokerURL/PairingToken`；只有 `easyGet ssh` 用 `flags.broker`。
   spike 期间因此有 3 次测试请求打到了**生产** Broker（item `SPIKE_KEY`，全部 TTL 过期，无批准、无密钥释放）。
   本地测试请用 `EASY_UNLOCKER_BROKER_URL` / `EASY_UNLOCKER_PAIRING_TOKEN`，或写一份指向本地的 config 文件。
   这个 CLI bug 值得单独修（要么让两个入口都吃 flag，要么把 flag 从 `env` 上去掉，别留「看着生效其实没生效」）。
2. `wrangler dev` 会读 `.dev.vars`（已 gitignore）；别把 CF token / FCM json 写进去再提交。
3. 这个 spike 的 `.dev.vars` 是本地生成的假值，跟生产 pairing token 无关。

## 这里**没有**实现

| 没做 | 原因 |
|---|---|
| `ModeWrite`（服务端 `rbw get`） | Workers 里没有 rbw，也不该有。手机密封路径用不到它 |
| `ModeSign` 服务端签证书 | 服务端不持有 CA：sign 请求只透传（public_key/ssh_user/cert_ttl）给手机，证书由 App 本地签发密封回传 |
| ntfy 回退 | 目标就是把自托管 ntfy 一起下掉 |
| 审计落 R2 | 现在只 `console.log`（进 Workers Logs），不落库 |
| 自定义域名 | 可绑，`docs/DEPLOY.md` 方式一有步骤；默认 workers.dev 够用 |

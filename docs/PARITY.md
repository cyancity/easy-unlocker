# 双端需求树（Android / iOS / Broker / CLI）

> 功能增删改时必须同步本表——这是双端对齐的唯一事实源。iOS 已合并 main（`6c81e18`）。
>
> 图例：✅ 已实现 / 🚧 半成品（stub 或部分）/ ❌ 未实现 / ➖ 不适用

## 协议面（Broker）

| 能力 | Go broker | CF worker | 备注 |
|---|---|---|---|
| env 请求 → 批准 → 密封交付（boxpayload） | ✅ | ✅ | |
| list 模式（#items 手选） | ✅ | ✅ | |
| sign 模式（`public_key`/`ssh_user`/`cert_ttl`） | ✅ | ✅ | 手机端签 OpenSSH 证书 |
| 设备角色（approver/requester） | ✅ | ✅ | |
| 配对码（pair-code → claim） | ✅ | ✅ | 绑租户 |
| 设备列表/撤销/续期 | ✅ | ✅ | 租户内 |
| 设备改名 `POST /v1/device/rename` | ✅ | ✅ | 租户内，approver |
| `GET /v1/version`（release tag 上报） | ✅ | ✅ | worker 报 wrangler vars 的 `CLI_VERSION`，发版后需同步 |
| **多租户（vault_id）** | ✅ | ✅ | 同租户 approver 独占；老记录归 default |
| FCM 推送 | ✅ | ✅ | 按租户过滤 |
| 审计带 tenant | ✅ | ✅ | |

## App 功能面

| 能力 | Android | iOS | 备注 |
|---|---|---|---|
| 建库/解锁（指纹+密码+恢复码） | ✅ | ✅ | iOS 生物识别按硬件自适应 |
| 条目 CRUD/搜索 | ✅ | ✅ | |
| 批准闭环（env/list/sign 三模式） | ✅ | ✅ | 请求卡均含还原命令块（commandLine） |
| sign 批准页（登录用户/有效期/公钥指纹/选 CA） | ✅ | ✅ | |
| 编辑页一键生成 CA 私钥填备注 | ✅ | ✅ | |
| 详情页识别私钥→展示 CA 公钥 | ✅ | ✅ | |
| 多网关（配对/切换/改名/移除） | ✅ | ✅ | 网关改名为本地标签 |
| 网关配对携带 vault_id | ✅ | ✅ | |
| 库初始化生成 vault_id + 导出/导入沿用 | ✅ | ✅ | 老库解锁时补写并落盘 |
| 设备管理：列表（只读） | ✅ | ✅ | |
| 设备管理：配对码/撤销/续期 | ✅ | ✅ | |
| 设备管理：改名 | ✅ | ✅ | /v1/device/rename |
| 推送：FCM / APNs | ✅ | ❌ | iOS APNs 未接入：个人团队无 Push capability，轮询兜底；待付费账号验真 |
| 密码批准兜底（无指纹时） | ✅ | ✅ | |
| 批准记录（筛选 + 详情页） | ✅ | ✅ | 详情页字段 via/field/took_ms/command_line 两端一致；iOS via 细分 faceid/touchid/opticid |
| 导出密文备份 / 导入（含 Bitwarden） | ✅ | 🚧 | Android 导出（保存到文件/分享）+ .eu1 整库导入 + Bitwarden 导入全通；iOS 有导出和 Bitwarden 导入，缺 .eu1 导入 |
| FLAG_SECURE 截屏保护 | ✅ | ➖ | iOS 原生截屏行为不同 |

## CLI

| 能力 | 状态 |
|---|---|
| env / list / sign 请求 | ✅ |
| `--ssh-user`/`--cert-ttl`/`--identity` | ✅ |
| `pair --code`（绑租户） | ✅ |
| 显式传租户 | ➖ 设计上不需要（token 反查） |
| `version` 子命令 + ping 报新版本 | ✅ | 版本经 release.sh `-X main.version=$TAG` 注入；`env/ssh/list` 成功后一天一次静默检查（`~/.config/easy-unlocker/.version-check`） |

## 已知问题 TODO（双端通用）

1. ~~可重连长轮询~~ ✅ **已实现**（`feat/reconnectable-request`）：`request_key` 幂等键 + detached 15s 宽限期 + CLI 自动重连；`--no-proxy`/`EASYGET_NO_PROXY` 直连开关
2. **前台多网关不可见**：App 无 `FirebaseMessagingService`——前台时 FCM data 静默丢弃、通知不进系统栏，只有后台通知被点击才 `markFromNotification` 切网关。待补：前台收 push 的 service + 「其他网关有待批准」角标（轮询所有已配网关）

## 当前 iOS 对齐缺口

1. **APNs 推送未接入**：个人免费团队不支持 Push capability，开发包摘 `aps-environment`；代码路径（pushToken 上报 + 1s/3s 轮询兜底）在但通道未验真，待付费开发者账号。
2. 编辑/删除条目、sign 证书 UI 流程、恢复码抄写的 UITest 用例未覆盖（crypto 层已对拍验证）。

## 已知交互差异（低优先级，不强制对齐）

- 拒绝请求后：iOS 出「已拒绝」结果页；安卓 toast + 回条目页。
- 待批准刷新：iOS 下拉刷新；安卓靠 tab 点按刷新 + 离线横幅重试。

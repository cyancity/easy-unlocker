# easy-unlocker Android App（P1）

需求真源：[docs/P1-APP.md](../docs/P1-APP.md)。  
**Agent 构建/安装**：[docs/AGENTS-SETUP.md](../docs/AGENTS-SETUP.md)。

## 打开

用 Android Studio 打开本目录 `android/`，JDK 17，Sync 后 Run。

## 第一次使用

1. 创建库：恢复码会写成手机 Documents 下的 `easy-unlocker-recovery.txt`，平时用指纹解锁，不用手打恢复码。
   已有旧库的：用恢复码解锁一次，之后就会改成指纹。
2. 添加一条 Agent 凭据（名称可以和 `easyGet` 请求名不同）。
   要批量搬的：Bitwarden → 工具 → 导出 → 选 **`.json`（未加密）**，然后在 App「设置 → 导入 Bitwarden 导出」里选这个文件，搜索/勾选要的条目再点导入。只认登录条目，取的名称、用户名和密码；`notes`/TOTP/自定义字段不读。导入的明文只走内存，不会落盘。
3. 配对：Broker `https://broker.example.com`，pairing token 只在自己终端取：

```bash
ssh your-server "grep '^EASY_UNLOCKER_PAIRING_TOKEN=' ~/easy-unlocker/.env"
```

4. 不要把 token 贴回聊天。配对成功后 App 只保留设备令牌。

**多网关**：设置 → 网关 → 右上角 + 可以再添加一台（例如 CF worker），列表里点「切换」随时换。
网关只决定「往哪儿收/批请求」，**切换不会动本地条目**；同一 URL 再配一次是更新那台的令牌。
通知里带了网关标识，点通知会自动切到发请求的那台。

后台系统通知改 Google FCM 的步骤见 [docs/FCM.md](../docs/FCM.md)。没配之前，**App 开在前台会每 3 秒拉一次待批准**，新请求会自己跳到待批准页。

> ⚠️ 从 git worktree 里编译时记得先 `cp ../android/app/google-services.json app/`：这个文件被 gitignore，不会跟着 worktree 走。缺了它 App 照样能跑（前台轮询），但**完全收不到 FCM 通知**，也不会有任何报错提示。

## 批准

电脑上：

```bash
easyGet env OPENAI_API_KEY --write-to /tmp/t.env --for "冒烟"
```

手机收到「请打开 App」（通知上没有批准按钮，密钥只在手机里）。打开 App → 指纹 → 手选条目 → 批准。

`easyGet env` 每次现场生成临时公钥；Broker 只转发密文。

## 备份

「设置 → 导出密文备份」得到密文文件，可保存到文件管理器或分享出去。换机/迁移：新装首屏或「设置 → 导入密文备份」选这个 `.eu1` 文件、输它的恢复码即可——整库覆盖，不是合并；导入后解锁密码要重设，指纹会自动重包。

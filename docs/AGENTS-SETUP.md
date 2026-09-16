# Agent 安装与配置

> 本文给编码助手。人读项目说明请看仓库根 [`README.md`](../README.md)。
>
> **禁止**：把 pairing token、恢复码、密钥、`google-services.json`、FCM 服务账号写进对话、commit、日志或 `--for` 以外的命令输出。取 token 时用命令替换，不要 `cat` 后回显。
>
> **不要做**（除非用户这轮明确要求）：改 `main`、部署/重启 Broker、改 `Notifications.CHANNEL_ID`（当前 `unlock_v2`）、改 Broker 协议或密码学、给 iOS、接 Bitwarden/autofill。
>
> **双端规则**：Android/iOS 双端并行开发。任何功能面或协议面的增删改，必须同 commit 或紧随更新 [`PARITY.md`](PARITY.md)（双端需求树）——一端先做了就标对面缺口，保证 iOS 追进度时能看到差距。Broker 双实现（Go + CF worker）同理，见 PLAN.md 坑16。

Broker 用你部署的那台（自建见 [`DEPLOY.md`](DEPLOY.md)），本文统一写 `https://broker.example.com`。新功能在仓库内 `.worktree/<slug>` 开分支，不要直接改 `main`。

---

## 1. 装 `easyGet`

### 1.1 直接下 release（**默认走这条**）

不要为了拿一个二进制去 clone + 编译。每次 main 合并都会打 release。

```bash
os=$(uname -s | tr 'A-Z' 'a-z')
arch=$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')
curl -L -o /tmp/easyGet "https://github.com/cyancity/easy-unlocker/releases/latest/download/easyGet-${os}-${arch}"
install -m 755 /tmp/easyGet ~/.local/bin/easyGet
easyGet --help          # 英文说明；能打出来即为当前版本
```

指定版本把 `latest/download` 换成 `download/<tag>`。Windows 下 `easyGet-windows-amd64.exe`。

同一 release 里还有 `broker-linux-arm64`（部署用）、`easy-unlocker-debug.apk`（手机用）、`SHA256SUMS`（校验）。

装完照 2 节写配置。手机装 APK：

```bash
curl -L -o /tmp/easy-unlocker.apk "https://github.com/cyancity/easy-unlocker/releases/latest/download/easy-unlocker-debug.apk"
adb install -r /tmp/easy-unlocker.apk    # 装完杀掉 App 再打开
```

### 1.2 从源码编译（改代码时）

需要本机 Go。二进制 **不要提交**（`dist/` 已 gitignore）。

```bash
cd <repo>
make build
# 或：
go build -trimpath -o dist/easyGet ./cli/cmd/easyget
```

确认是当前源码编出来的（旧二进制没有 `seal_public_key`，手机会报「没有传输公钥」）：

```bash
./dist/easyGet ping
# 未配置时会连默认 127.0.0.1:8787，失败也没关系，至少二进制能跑
```

可选：`./scripts/install.sh` 会把 `easyGet` 装到 `~/.local/bin/easyGet`，并写配置。需要环境变量 `EASY_UNLOCKER_BROKER_URL` 和 `EASY_UNLOCKER_PAIRING_TOKEN`。它还会往**当前工作目录**的 `AGENTS.md` 追加一段「需要凭据时只调用 easyGet」——那是给**调用方项目**的，不是本仓库的本文。

### 1.3 发版（维护者）

**main 每次合并后跑一次**，否则别的机器只能现场编译：

```bash
bash scripts/release.sh            # 打当天 tag vYYYY.MM.DD，同日再打是 -2、-3
bash scripts/release.sh --no-apk   # 跳过 gradle（只发 easyGet / broker）
```

会交叉编译 `easyGet`（darwin/linux/windows × arm64/amd64）、`broker-linux-arm64`、APK，附 `SHA256SUMS`，用 `gh release create --generate-notes` 发布。要求：在 main、工作区干净、与 origin 一致、`gh` 已登录；打 APK 还要求 `android/app/google-services.json` 与 `android/local.properties` 存在。

不在 CI 里做的原因：APK 必须带 `google-services.json`（gitignore），且 CI 的 debug 签名与你本机不同，`adb install -r` 会签名冲突。

---

## 2. 配置 CLI

`easyGet` 读配置的顺序：`--config` → 环境变量 `EASY_UNLOCKER_CONFIG` → `~/.config/easy-unlocker/config`。环境变量会覆盖文件：

| 变量 | 作用 |
|---|---|
| `EASY_UNLOCKER_BROKER_URL` | 你的 Broker，如 `https://broker.example.com` |
| `EASY_UNLOCKER_PAIRING_TOKEN` | 配对令牌，只存在本机 |
| `EASY_UNLOCKER_REQUESTER` | 可选；默认本机用户名@主机名 |
| `EASY_UNLOCKER_ALLOW_HTTP` | 可选；设为 `1` 才允许非本机的 `http://` Broker（默认只接受 https，本机 127.0.0.1/localhost 除外） |

Broker 地址必须是 `https://`；`http://` 只对本机地址放行，否则设备令牌会明文过网。

配置文件权限必须是 `0600`（其它用户位必须为 0，否则 `easyGet` 拒绝读取）。目录建议 `0700`。

```bash
umask 077
mkdir -p ~/.config/easy-unlocker
chmod 700 ~/.config/easy-unlocker

# pairing token：在你部署 Broker 的 .env 里。只在自己终端取值，不要打印、不要贴进对话
# 推荐写法：手机 App 生成配对码，走 easyGet pair（见下）；下面是手动写配置的兜底
umask 077
cat > ~/.config/easy-unlocker/config <<EOF
broker_url=https://broker.example.com
pairing_token=<你的 pairing token>
EOF
chmod 600 ~/.config/easy-unlocker/config

./dist/easyGet ping
# 成功：正常：Broker 可达
```

更省事的路：不用手动搬 pairing token——手机 App → 设置 → 设备 → 生成配对码，然后 `easyGet pair --broker <url> --code <码>` 会自己写好这份配置（拿到的是 requester 设备令牌，权限更小）。

token 拿不到时停下来问用户，不要扫全盘找。

### 发一条请求

`--for` 必需，并且**必须选一种放出方式**：`--write-to`（落盘）或 `--exec`（不落盘），二者互斥。默认 TTL 300 秒，上限 3600。

```bash
# 1) 落盘：值写进文件，会留在本机
./dist/easyGet env MY_API_KEY \
  --write-to /tmp/easy-unlocker.env \
  --for '说明用途' \
  --ttl 3600

# 2) 不落盘：值只交给子进程
./dist/easyGet env MY_SERVICE_TOKEN \
  --exec 'python run.py' \
  --for '跑一次性脚本'
```

**怎么选**（手机批准页会把后果写给你看，用户据此确认）：

| 情形 | 用哪个 |
|---|---|
| 程序从环境变量读密钥；跑完就不要了 | `--exec`（不落盘） |
| 需要给程序一个**文件路径**（oci 的 `key_file`、`ssh -i` 这类） | `--exec` + `$EASYGET_SECRET_FD` |
| 确实要一个文件留在本机（如给别的进程读的 `.env`） | `--write-to`（会落盘） |
| 私钥类（PEM、私钥文件） | 优先 `--exec`；非要落盘时让用户知道 |

`--exec` 里子进程能拿到：

| 变量 | 内容 |
|---|---|
| `EASYGET_SECRET` | 值的本体；用 `--env-name MY_KEY` 可改名 |
| `EASYGET_SECRET_FD` | `/dev/fd/3`，指向一个**匿名 fd**（写完就 unlink），给必须吃文件路径的工具 |
| `EASYGET_ITEM` | 这次的条目名 |

`--exec` 会**原样透传子进程退出码**。值写进临时载体后立刻 unlink，文件系统里搜不到；但 macOS 没有 memfd，物理磁盘可能留有已释放块——不声称「绝对不过磁盘」。

落盘时成功只打印 `已完成`，值在 `--write-to` 文件里（`0600`）。不要 `cat` 该文件到对话。

拒绝 / 过期 / 失败：非零退出。公网长轮询大约 2 分钟会被掐，终端可能报 `Broker 请求失败`——这是已知限制；手机待批准会在约 1 秒内一起消失。要批完就在掐线前在手机上点。

条目名只是请求提示，不必和库里名称相同；对不上时用户在手机上手选。

### 不知道条目名时

`easyGet list` 打本机缓存的条目名（只有名字和别名，没有任何值），不需要批准、不联网。stdout 一行一条、别名跟在括号里；缓存时间和陈旧提示走 stderr。

```bash
./dist/easyGet list
# my-api-key (OPENAI_API_KEY)
# my-service-token
```

**缓存是告示牌，不是账本。** 手机上的库才是唯一权威，它随时可能加过、删过、改过名：

- 用户口头给你的名字，**直接请求**。名字不在名单里**不等于**条目不存在，不要据此回「没有这个条目」。
- 要一份新的用 `easyGet list --refresh`。那是一次真请求：手机会弹通知、要一次指纹，所以别每次取用前都刷。
- 缓存文件 `~/.config/easy-unlocker/items.json`（`0600`，与 `config` 同目录，跟随 `--config` / `EASY_UNLOCKER_CONFIG`）。删掉它不影响取件。
- `#items` 是刷新名单用的保留条目名，不能当条目名或别名。

> **`--refresh` 需要新版 App。** 老 App 不认识 `#items`，会把它当普通请求渲染，用户手选一条凭据点了批准就会放出一条真值；CLI 会把这种情况认出来并拒绝——**不落盘、不打印**，只报错，但那次批准白费了。先升级手机上的 App 再用 `--refresh`；只读缓存的 `easyGet list` 没有这个前提。

### SSH 登录（短时证书）

```bash
./dist/easyGet ssh my-ssh-ca --for '登录服务器跑诊断'
# 手机批准 → 证书 + 临时身份落 ~/.ssh/
ssh -i ~/.ssh/easy-unlocker-my-ssh-ca user@host
```

- `<ca-item>` 是库里**当 CA 用的那条条目**（备注/密码栏里是一把 OpenSSH ed25519 私钥），不是目标主机名；对不上照旧手选。
- 手机签的是**证书**，CA 私钥永远不出手机；目标机要先把 CA 公钥写进 `TrustedUserCAKeys`。
- `--ssh-user`（默认本机用户名）指定登录账号；`--cert-ttl`（默认 300，上限 86400）指定证书有效期。**证书只在 SSH 握手那一刻校验**——已建立的会话不会因为证书过期而断开，`--cert-ttl` 是「批准后多久内要发起登录」的窗口，不是会话最长时长。
- 证书不是秘密但 CA 私钥是：不要 `cat` 任何私钥文件。

> **`easyGet ssh` 需要新版 App。** 老 App 会把 sign 请求当普通放出渲染——用户若手选到 CA 条目点批准，**CA 私钥本身会被封出去**。升级 App 前不要跑这个命令。

---

## 3. 构建并安装 App

### 3.1 Android

包名：`io.github.cyancity.easyunlocker`。用 Android Studio **打开 `android/` 目录**（不是仓库根）。JDK 17，minSdk 28。

锁屏通知需要 `android/app/google-services.json`（gitignore）：Firebase 控制台里加一个**包名等于 `io.github.cyancity.easyunlocker`** 的 Android 应用，下载的 json 放这个位置。包名对不上 `processDebugGoogleServices` 会直接报错；缺这个文件 App 仍能用，但没有后台/锁屏通知，只能前台轮询。详见 [`FCM.md`](FCM.md)，**不要把 JSON 贴进对话或提交**。

```bash
test -s android/app/google-services.json && echo 'google-services: present'
```

`android/local.properties` 也 gitignore，内容一行即可：

```
sdk.dir=/Users/<you>/Library/Android/sdk
```

命令行构建：

```bash
cd android
./gradlew :app:assembleDebug
adb devices -l
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

覆盖安装后请用户杀掉再打开一次。APK 和 `dist/` 都不入库。

### 3.2 iOS

bundle id：`io.github.cyancity.easyunlocker`。最低部署 iOS 17。

```bash
cd ios
python3 generate-project.py     # 扫描目录重生成 project.pbxproj；加了源码文件后重跑一次
open EasyUnlocker.xcodeproj     # 用 Xcode 打开
```

`DEVELOPMENT_TEAM` 默认留空——真机构建时在 Signing & Capabilities 里选你自己的 Team；模拟器不需要。真机第一次装要信任开发者证书。APNs 推送代码在但个人免费团队没有 Push capability，没接也没验过；App 开着时靠轮询兜底。

UI 测试：`EasyUnlockerUITests`（`ios/EasyUnlocker.xctestplan`）。测试里的 broker 常量默认 `127.0.0.1:8787`（模拟器走宿主机回环）；真机验收改成跑测试 Broker 那台机器的 LAN IP。真机注意「保存密码」系统弹窗和 sheet 焦点残留，现有用例里都有处理样例。

### 3.3 第一次使用（指导用户，不要代填 token）

1. 创建库：恢复码写到本机 Documents 下 `easy-unlocker-recovery.txt`。日常指纹，不要让用户把恢复码发给你。
2. 添加一条凭据（名称可与 `easyGet` 的 item 不同）。
3. 设置 → 配对：Broker 填你的 `https://broker.example.com`；pairing token 让用户在**自己终端**从 Broker `.env` 里取，不要经聊天转发。
   - 设备分两种角色：用 pairing token 配对的（手机）是 **approver**，能批准、管设备；用配对码 `easyGet pair` 换来的（CLI 机器）是 **requester**，只能发起请求。CLI 机器不要用 pairing token 直连，配对码换令牌才是最小权限。
4. Android 端收不到推送时排查：通知权限、电量策略「无限制」、允许自启动——部分 ROM 默认杀后台，杀掉 App 后锁屏收不到 FCM。

通知渠道 id 是 `unlock_v2`。声音和锁屏可见性创建后改不了，**不要换 id**。

设置里还有：批准后自动关闭（默认 3 秒关掉 App）、批准记录（只在本机，不存密钥）。

---

## 4. 冒烟（有手机时）

1. App 开在前台。
2. 电脑跑第 2 节的 `easyGet env`。
3. 手机渐入待批准；对不上则手选 → 批准并放出 → 指纹。
4. 终端 `已完成`，不要读目标文件内容。
5. 设置 → 批准记录里应有一条「已放行」。

锁屏路径：杀掉 App、锁上屏幕，再发 `easyGet`；应出现「easy-unlocker / 有一条待批准的请求」。

---

## 5. 相关文件

| 文件 | 给谁 |
|---|---|
| [`README.md`](../README.md) | 人 |
| 本文 | Agent 装 CLI / App |
| [`P1-APP.md`](P1-APP.md) | 需求真源 |
| [`FCM.md`](FCM.md) | Firebase 控制台步骤 |
| [`DEPLOY.md`](DEPLOY.md) | 仅当用户要求部署 Broker |
| [`android/README.md`](../android/README.md) | Studio 打开方式（短） |
| [`ios/README.md`](../ios/README.md) | Xcode 构建方式（短） |

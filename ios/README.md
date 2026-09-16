# easy-unlocker iOS App

功能对齐 Android 版，UI 用 Apple 原生风格。需求与双端差距见 [docs/PARITY.md](../docs/PARITY.md)。
**Agent 构建/安装**：[docs/AGENTS-SETUP.md](../docs/AGENTS-SETUP.md)。

## 构建

需要 Xcode，最低部署 iOS 17。

```bash
cd ios
python3 generate-project.py   # 扫描目录生成 project.pbxproj；加了源码文件后重跑一次
open EasyUnlocker.xcodeproj
```

- `DEVELOPMENT_TEAM` 默认留空：真机构建在 Signing & Capabilities 里选你自己的 Team，模拟器不用管。
- bundle id 是 `io.github.cyancity.easyunlocker`，要换自己的标识就全局搜替换，并同步 `EasyUnlocker.entitlements` 里的 keychain group。
- 真机第一次装要在系统设置里信任开发者证书。

## 使用

1. 创建库：恢复码写到本机文件，平时用 Face ID / Touch ID / 密码解锁。
2. 添加条目，或从 Bitwarden 未加密 `.json` 勾选导入。
3. 设置 → 网关 → 配对：Broker 填你的地址（自建见 [docs/DEPLOY.md](../docs/DEPLOY.md)），pairing token 在自己终端从 Broker `.env` 里取，不要贴回聊天。
4. 批准后 App 退回条目页（iOS 不允许进程自杀）；批准记录只存本机。

## 推送

APNs 未接入：个人免费开发者团队没有 Push capability，推送代码在但通道未验真。现在靠 App 打开时的轮询兜底——收到请求时 App 不在前台就看不到，打开即同步待批准列表。

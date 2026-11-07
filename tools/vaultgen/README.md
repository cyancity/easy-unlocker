# vaultgen —— 给模拟器造一条测试用凭据

模拟器里跑 UI 测试时，手工点「创建保险库 → 填恢复码 → 录指纹」既慢又不稳。这个工具
直接生成一个 `vault.eu1` 塞进模拟器的数据容器，省掉整套建库流程。

格式逐字对齐 `android/.../crypto/VaultCrypto.kt` 与 `ios/.../Crypto/VaultCrypto.swift`：
`argon2id(recovery16B, salt, ops=3, mem=64MiB, p=1)` → `AES-256-GCM(AAD "easy-unlocker-vault")`。
生成的库两端 App 都能用。

## 用法

```bash
go run ./tools/vaultgen aaaa-bbbb-cccc-dddd-eeee-ffff-0000-1111 /tmp/vault.eu1
```

恢复码必须是 32 位 hex（可带短横线）。工具会把条目写死成一条：

| 栏 | 值 |
|---|---|
| 名称 | `my-token` |
| 密码 | `short-password-value` |
| 备注 | `long-note-value-in-note-field` |

两栏都非空——这正是「值在备注里，但条目也有密码」的场景，`--field note` 要验的就是它。

## 塞进模拟器

**iOS**（容器路径在宿主机文件系统上，直接拷）：

```bash
DEV=<模拟器 UDID>
C=$(xcrun simctl get_app_container $DEV io.github.cyancity.easyunlocker data)
DIR="$C/Library/Application Support/easy-unlocker"
mkdir -p "$DIR"
cp /tmp/vault.eu1 "$DIR/vault.eu1"
printf "aaaa-bbbb-cccc-dddd-eeee-ffff-0000-1111\n" > "$DIR/recovery.txt"
```

`recovery.txt` 是 App 存恢复码的地方；App 解锁时先读它，就不用再手输。

**Android**（同理，容器在 `/data/data/<包名>/files/`，需要 root 或 `run-as`）：

```bash
adb push /tmp/vault.eu1 /data/local/tmp/
adb shell run-as io.github.cyancity.easyunlocker cp /data/local/tmp/vault.eu1 files/vault.eu1
adb shell run-as io.github.cyancity.easyunlocker sh -c \
  'echo aaaa-bbbb-cccc-dddd-eeee-ffff-0000-1111 > files/recovery.txt'
```

Android 的恢复码也在 `filesDir`（`files/recovery.txt`），跟 iOS 一样。

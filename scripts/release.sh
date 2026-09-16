#!/usr/bin/env bash
# 打 release：easyGet 各平台二进制 + broker(linux/arm64) + Android APK，挂到 GitHub Release。
#
# 为什么要有这个：其他 agent 和别的机器不该现场编译，直接下 release 产物。
# 为什么不做 CI：APK 必须带 android/app/google-services.json（gitignore，CI 里没有），
#   而且 CI 的 debug 签名与你要覆盖安装的本机包不同，`adb install -r` 会签名冲突。
#
# 用法：
#   bash scripts/release.sh                 # 打当天 tag（vYYYY.MM.DD，同日再加 -2、-3）
#   bash scripts/release.sh --tag v2026.09.20
#   bash scripts/release.sh --no-apk        # 只发 easyGet / broker（跳过 gradle）
#
# 前置：在 main、工作区干净、与 origin 一致；gh 已登录；打 APK 需要 Android SDK、
#      android/app/google-services.json、android/local.properties。
set -euo pipefail

REPO="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO"

TAG=""
WITH_APK=true
while [ $# -gt 0 ]; do
  case "$1" in
    --tag) TAG="${2:-}"; shift 2 ;;
    --no-apk) WITH_APK=false; shift ;;
    -h|--help) sed -n '2,16p' "$0"; exit 0 ;;
    *) echo "未知参数: $1（用 --help）" >&2; exit 2 ;;
  esac
done

fail() { echo "release: $*" >&2; exit 1; }

# ---------- 前置检查 ----------
[ "$(git rev-parse --abbrev-ref HEAD)" = "main" ] || fail "只能在 main 上打 release（当前 $(git rev-parse --abbrev-ref HEAD)）"
[ -z "$(git status --porcelain)" ] || fail "工作区不干净，先提交或 stash"
git fetch --quiet origin
[ "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" ] || fail "本地 main 与 origin/main 不一致，先 push/pull"
command -v gh >/dev/null || fail "需要 gh（GitHub CLI）"
command -v go >/dev/null || fail "需要 Go 工具链"
if $WITH_APK; then
  [ -f android/app/google-services.json ] || fail "缺 android/app/google-services.json（FCM 配置，gitignore，必须本机存在）"
  [ -f android/local.properties ] || fail "缺 android/local.properties（sdk.dir）"
fi

# ---------- tag ----------
if [ -z "$TAG" ]; then
  base="v$(date +%Y.%m.%d)"
  TAG="$base"
  n=1
  while git rev-parse -q --verify "refs/tags/$TAG" >/dev/null 2>&1; do
    n=$((n + 1))
    TAG="$base-$n"
  done
fi
git rev-parse -q --verify "refs/tags/$TAG" >/dev/null 2>&1 && fail "tag $TAG 已存在"

OUT="dist/release/$TAG"
rm -rf "$OUT"
mkdir -p "$OUT"

# ---------- easyGet：多平台 ----------
echo "== 编译 easyGet =="
for target in darwin/arm64 darwin/amd64 linux/arm64 linux/amd64 windows/amd64; do
  os="${target%%/*}"
  arch="${target##*/}"
  name="easyGet-$os-$arch"
  [ "$os" = "windows" ] && name="$name.exe"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "-s -w -X main.version=$TAG" -o "$OUT/$name" ./cli/cmd/easyget
  echo "  $name"
done

# ---------- broker：只发服务器真正跑的那个平台 ----------
echo "== 编译 broker =="
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -buildvcs=false -ldflags "-s -w -X main.version=$TAG" -o "$OUT/broker-linux-arm64" ./broker/cmd/broker
echo "  broker-linux-arm64"

# ---------- Android APK ----------
if $WITH_APK; then
  echo "== 编译 APK（gradle，可能要几分钟）=="
  (cd android && ./gradlew --console=plain :app:assembleDebug | tail -5)
  apk="$(ls -t android/app/build/outputs/apk/debug/*.apk 2>/dev/null | head -1)"
  [ -n "$apk" ] || fail "没找到 APK 产物"
  cp "$apk" "$OUT/easy-unlocker-debug.apk"
  echo "  easy-unlocker-debug.apk ($(du -h "$OUT/easy-unlocker-debug.apk" | cut -f1))"
else
  echo "== 跳过 APK（--no-apk）=="
fi

# ---------- 校验和 ----------
if command -v shasum >/dev/null; then
  (cd "$OUT" && shasum -a 256 ./* >SHA256SUMS)
else
  (cd "$OUT" && sha256sum ./* >SHA256SUMS)
fi

# ---------- 发布 ----------
# 本地把 notes 拼好，只发一次 POST：`--generate-notes` + `--notes` 会让 gh 建完再
# PATCH 一次补 notes，那条 PATCH 在代理链路上被掐过（EOF），release 白建。
echo "== 创建 release $TAG =="
names=""
for f in "$OUT"/*; do
  names="$names $(basename "$f")"
done
prev="$(git describe --tags --abbrev=0 2>/dev/null || true)"
if [ -n "$prev" ]; then
  commits="$(git log --oneline --no-merges "$prev..HEAD" | head -40)"
  range="自 $prev 起"
else
  commits="$(git log --oneline --no-merges -20)"
  range="最近提交"
fi
notes="产物：$names

${range}：
$commits"
gh release create "$TAG" "$OUT"/* --title "$TAG" --notes "$notes" >/dev/null
echo
echo "✅ $TAG 已发布：$(gh release view "$TAG" --json url -q .url)"
echo
echo "其他机器装 easyGet（不用编译）："
echo "  os=\$(uname -s | tr 'A-Z' 'a-z'); arch=\$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')"
echo "  curl -L -o /tmp/easyGet \"https://github.com/cyancity/easy-unlocker/releases/latest/download/easyGet-\${os}-\${arch}\""
echo "  install -m 755 /tmp/easyGet ~/.local/bin/easyGet && easyGet --help"
echo "手机装 APK："
echo "  curl -L -o /tmp/easy-unlocker.apk https://github.com/cyancity/easy-unlocker/releases/latest/download/easy-unlocker-debug.apk"
echo "  adb install -r /tmp/easy-unlocker.apk"

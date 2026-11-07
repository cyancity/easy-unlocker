#!/usr/bin/env bash
# emulator.sh —— 起 easy-unlocker 的安卓模拟器，用宿主 GPU，不吃 CPU。
#
# 用法：source scripts/emulator.sh start|stop|status|shell
#
# 为什么是这套参数（Windows 宿主的实测值，2026-11-07）：
#   -gpu host        走宿主显卡（RTX 3070）。SwiftShader 是 CPU 软渲染，冷启动
#                    能把宿主 CPU 打满——这就是之前 100% 占用的来源。
#                    起来后用 dumpsys SurfaceFlinger | grep GLES 确认是
#                    "Google (NVIDIA Corporation) ... RTX 3070" 才算走通。
#   -cores 2         冷启动峰值从 421% 单核降到 237%。模拟器跑 UI 测试用不了
#                    这么多核，限制住才不会跟宿主其它活抢 CPU。
#   -memory 2048     AVD 默认 1536M 偏紧，界面动画容易卡。
#   -no-snapshot     每次冷启动，避免快照里带坏状态（也避免"这次怎么这么快"
#                    的困惑）。慢一点但可复现。
#
# 宿主加速：WHPX 已装（emulator -accel-check 会报 usable），所以 x86_64 镜像
# 是硬件虚拟化跑的，不是纯软件模拟。
#
# SDK 位置：ANDROID_SDK_ROOT 或本机默认的临时目录，两者都没有就先装——
# 见本文件末尾的"装环境"。

set -u

: "${EU_AVD:=eu_test}"
: "${EU_PORT:=5554}"
: "${EU_SDK:=${ANDROID_SDK_ROOT:-${ANDROID_HOME:-}}}"
if [ -z "$EU_SDK" ]; then
	for candidate in "$HOME/AppData/Local/Temp/android-sdk" "$HOME/Android/Sdk" "$HOME/Library/Android/sdk"; do
		if [ -d "$candidate" ]; then EU_SDK="$candidate"; break; fi
	done
fi

if [ -z "$EU_SDK" ] || [ ! -d "$EU_SDK" ]; then
	echo "找不到 Android SDK。先装一个，或者设 ANDROID_SDK_ROOT 指向已有的。" >&2
	echo "装环境见本文件末尾。" >&2
	return 1 2>/dev/null || exit 1
fi

export ANDROID_HOME="$EU_SDK"
export ANDROID_SDK_ROOT="$EU_SDK"
ADB="$EU_SDK/platform-tools/adb.exe"
[ -x "$ADB" ] || ADB="$EU_SDK/platform-tools/adb"
EMU="$EU_SDK/emulator/emulator.exe"
[ -x "$EMU" ] || EMU="$EU_SDK/emulator/emulator"

# 起模拟器：GPU 走宿主，核数压到 2，不建快照。
emulator_start() {
	if "$ADB" devices 2>/dev/null | grep -q "emulator-$EU_PORT.*device"; then
		echo "模拟器已在跑（emulator-$EU_PORT）"
		return 0
	fi
	echo "起模拟器 $EU_AVD（GPU=host, 2 核）……"
	# -no-window：后台跑，不弹窗。要看界面就去 Android Studio 里开 AVD。
	nohup "$EMU" -avd "$EU_AVD" \
		-no-window -no-audio -no-snapshot -no-boot-anim \
		-gpu host -cores 2 -memory 2048 \
		-port "$EU_PORT" \
		>"$EU_SDK/emulator-$EU_AVD.log" 2>&1 &
	echo "等启动……"
	for _ in $(seq 1 60); do
		if "$ADB" shell getprop sys.boot_completed 2>/dev/null | tr -d '\r' | grep -q '^1$'; then
			echo "起来了：emulator-$EU_PORT"
			# 确认真的走的是宿主 GPU，不是退回了软渲染。
			"$ADB" shell dumpsys SurfaceFlinger 2>/dev/null | grep -m1 '^GLES:' || true
			return 0
		fi
		sleep 5
	done
	echo "启动超时；看日志 $EU_SDK/emulator-$EU_AVD.log" >&2
	return 1
}

emulator_stop() {
	"$ADB" emu kill >/dev/null 2>&1 || true
	echo "已停"
}

emulator_status() {
	"$ADB" devices 2>/dev/null | grep -E "emulator-$EU_PORT" || echo "没在跑"
	"$ADB" shell dumpsys SurfaceFlinger 2>/dev/null | grep -m1 '^GLES:' || true
}

case "${1:-start}" in
	start) emulator_start ;;
	stop) emulator_stop ;;
	status) emulator_status ;;
	shell) shift; exec "$ADB" shell "$@" ;;
	*) echo "用法：source scripts/emulator.sh start|stop|status|shell" >&2; exit 1 ;;
esac

#!/usr/bin/env bash
# e2e-phone.sh —— easy-unlocker 真机验收的 adb UI 驱动函数库。
# 用法：source scripts/e2e-phone.sh；临时解锁密码放在 $PW_FILE（默认 /tmp/eu-acceptance/pw）。
# 设计：一律按「文本 → bounds 中点」定位，不硬编码坐标；键盘弹出会移动布局（imePadding），
# 所以所有输入前先收键盘再取坐标。

set -u
: "${PW_FILE:=/tmp/eu-acceptance/pw}"
: "${E2E_PKG:=io.github.cyancity.easyunlocker}"

# --- 基础 ---

ui_dump() {
    adb exec-out uiautomator dump /dev/tty 2>/dev/null && return
    adb shell uiautomator dump /sdcard/u.xml >/dev/null 2>&1 && adb shell cat /sdcard/u.xml
}

ui_text() { ui_dump | grep -o 'text="[^"]*"' | sed 's/^text="//;s/"$//' | grep -v '^$'; }

# 等待某文本出现；超时返回 1。用法：ui_wait '用密码批准' 15
ui_wait() {
    local pat="$1" timeout="${2:-15}" t=0
    while [ "$t" -lt "$timeout" ]; do
        ui_text | grep -q "$pat" && return 0
        sleep 1; t=$((t + 1))
    done
    return 1
}

_bounds_of() {  # $1=text 或 desc, $2=属性名
    ui_dump | tr '>' '\n' | grep "${2:-text}=\"$1\"" | grep -o 'bounds="[^"]*"' | head -1 |
        sed 's/bounds="\[\([0-9]*\),\([0-9]*\)\]\[\([0-9]*\),\([0-9]*\)\]"/\1 \2 \3 \4/'
}

tap_text() {  # tap 文本节点中点
    local b
    b=$(_bounds_of "$1" text) || return 1
    [ -z "$b" ] && b=$(_bounds_of "$1" content-desc)
    [ -z "$b" ] && return 1
    set -- $b
    adb shell input tap $((($1 + $3) / 2)) $((($2 + $4) / 2))
}

tap_desc() { tap_text "$1"; }  # desc 走 _bounds_of 的 fallback

# 收键盘（有键盘时布局会动，先收再定位才准）
kb_close() { adb shell input keyevent KEYCODE_BACK >/dev/null 2>&1; sleep 0.8; }

# 往「当前 focused」或指定序号的 EditText 输入；field_i 从 1 开始（屏幕上往下数）
type_field() {  # type_field <text> [field_i]
    local text="$1" idx="${2:-1}"
    kb_close
    local b
    b=$(ui_dump | tr '>' '\n' | grep 'class="android.widget.EditText"' |
        grep -o 'bounds="[^"]*"' | sed -n "${idx}p" |
        sed 's/bounds="\[\([0-9]*\),\([0-9]*\)\]\[\([0-9]*\),\([0-9]*\)\]"/\1 \2 \3 \4/')
    [ -z "$b" ] && return 1
    set -- $b
    adb shell input tap $((($1 + $3) / 2)) $((($2 + $4) / 2))
    sleep 0.5
    adb shell input text "$text"
    sleep 0.3
}

# 清空当前 focused 输入框：光标到开头再逐个前删（tap 落点决定光标位置，退格只删前面）
field_clear() {
    adb shell input keyevent KEYCODE_MOVE_HOME
    local i; for i in $(seq 40); do adb shell input keyevent KEYCODE_FORWARD_DEL; done
}

# --- 语义动作 ---

# 系统生物识别弹窗在就退掉（我们要走密码路径）
dismiss_biometric() {
    if ui_text | grep -q '请触摸指纹传感器\|使用 PIN 码'; then
        adb shell input keyevent KEYCODE_BACK >/dev/null 2>&1
        sleep 1
    fi
}

# 解锁页密码解锁；库已解锁则直接返回 0
unlock_by_password() {
    ui_text | grep -q '密码解锁' || return 0
    dismiss_biometric
    ui_wait '密码解锁' 10 || return 1
    type_field "$(cat "$PW_FILE")" 1
    kb_close
    tap_text '密码解锁' && sleep 4
}

# 批准页密码批准；返回非 0 = 页面没出现批准按钮（可能请求已过期/已处理）
approve_by_password() {
    ui_wait '用密码批准' "${1:-20}" || return 1
    tap_text '用密码批准'; sleep 1
    type_field "$(cat "$PW_FILE")" 1
    kb_close
    tap_text '确认批准' && sleep 4
}

# 打开 App 到前台（com.android monkey 方式不需要 component 名）
app_open() {
    adb shell monkey -p "$E2E_PKG" -c android.intent.category.LAUNCHER 1 >/dev/null 2>&1
    sleep 2
    dismiss_biometric
}

# 底栏切 tab：vault|pending|settings
go_tab() {
    case "$1" in
        vault)    tap_text '条目' ;;
        pending)  tap_text '待批准' ;;
        settings) tap_text '设置' ;;
    esac
    sleep 1.2
}

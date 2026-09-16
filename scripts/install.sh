#!/bin/sh

set -eu

install_dir=${EASYGET_INSTALL_DIR:-"$HOME/.local/bin"}
config_dir=${EASYGET_CONFIG_DIR:-"$HOME/.config/easy-unlocker"}
config_path=${EASYGET_CONFIG:-"$config_dir/config"}
agents_file=${EASYGET_AGENTS_FILE:-"$PWD/AGENTS.md"}
broker_url=${EASY_UNLOCKER_BROKER_URL:-}
pairing_token=${EASY_UNLOCKER_PAIRING_TOKEN:-}
source_binary=${EASYGET_BINARY:-}
download_url=${EASYGET_URL:-}
build_dir=
config_tmp=

cleanup() {
	if [ -n "$config_tmp" ]; then
		rm -f "$config_tmp"
	fi
	if [ -n "$build_dir" ]; then
		rm -rf "$build_dir"
	fi
}
trap cleanup EXIT HUP INT TERM

if [ -z "$broker_url" ] || [ -z "$pairing_token" ]; then
	echo "EASY_UNLOCKER_BROKER_URL 和 EASY_UNLOCKER_PAIRING_TOKEN 是必需的" >&2
	exit 1
fi

if [ -z "$source_binary" ]; then
	if [ -n "$download_url" ]; then
		build_dir=$(mktemp -d)
		source_binary="$build_dir/easyGet"
		curl --fail --silent --show-error --location "$download_url" --output "$source_binary"
	else
		repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
		if ! command -v go >/dev/null 2>&1 || [ ! -f "$repo_dir/go.mod" ]; then
			echo "请设置 EASYGET_BINARY（已有 easyGet 二进制）或 EASYGET_URL（发布地址）" >&2
			exit 1
		fi
		build_dir=$(mktemp -d)
		source_binary="$build_dir/easyGet"
		go build -trimpath -o "$source_binary" "$repo_dir/cli/cmd/easyget"
	fi
fi

if [ ! -f "$source_binary" ]; then
	echo "找不到 easyGet 二进制" >&2
	exit 1
fi

umask 077
mkdir -p "$install_dir" "$config_dir"
config_parent=$(dirname -- "$config_path")
mkdir -p "$config_parent"
chmod 700 "$config_dir"
cp "$source_binary" "$install_dir/easyGet"
chmod 755 "$install_dir/easyGet"

config_tmp=$(mktemp "$config_dir/config.XXXXXX")
chmod 600 "$config_tmp"
{
	printf '%s\n' "broker_url=$broker_url"
	printf '%s\n' "pairing_token=$pairing_token"
} >"$config_tmp"
mv -f "$config_tmp" "$config_path"
config_tmp=

marker_start='<!-- easy-unlocker:begin -->'
marker_end='<!-- easy-unlocker:end -->'
if [ ! -f "$agents_file" ] || ! grep -Fq "$marker_start" "$agents_file"; then
	{
		printf '\n%s\n' "$marker_start"
		printf '%s\n' '# easy-unlocker：需要凭据时只调用 easyGet，不要读取或打印凭据值。'
		printf '%s\n' '- API Key：easyGet env <item> --write-to <path> --for "说明用途"'
		printf '%s\n' '- SSH：easyGet ssh <name> --for "说明用途"'
		printf '%s\n' '- 不知道条目名：easyGet list（读本机缓存，免费、不联网）。要一份新的用 easyGet list --refresh（手机会要一次指纹），别每次取用前都刷。'
		printf '%s\n' '- 缓存只是上一次的快照：名字不在名单里不等于条目不存在，用户给的名字直接请求。'
		printf '%s\n' '- easyGet 的输出只有状态；不要把目标文件内容放入对话、日志或命令输出。'
		printf '%s\n' "$marker_end"
	} >>"$agents_file"
fi

PATH="$install_dir:$PATH" EASY_UNLOCKER_CONFIG="$config_path" "$install_dir/easyGet" ping
echo "easyGet 已安装到 $install_dir/easyGet"

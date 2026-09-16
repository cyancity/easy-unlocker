package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ReservedListItem 是 CLI 用来请求「列出条目名」的保留条目名。
//
// 复用 mode=write 是刻意的：Broker 对 mode 只认 sign|write（Go 与 worker 两份实现
// 都硬校验），加新 mode 或新字段都要同时重部署两个 Broker。手机看到这个名字就走
// 专用批准页，只回名字，不放任何值。
const ReservedListItem = "#items"

// itemListVersion 是名单的形状版本；两端必须一起改。
const itemListVersion = 1

// ItemEntry 是一条条目在名单里的样子：名字 + 别名。永远不含值。
type ItemEntry struct {
	Name    string   `json:"name"`
	Aliases []string `json:"aliases,omitempty"`
}

// ItemList 既是从手机封回来的一次快照，也是本机缓存的内容。
type ItemList struct {
	Version int `json:"v"`
	// FetchedAt（RFC3339）只有缓存有：名单是什么时候从手机取的。
	FetchedAt string      `json:"fetched_at,omitempty"`
	Items     []ItemEntry `json:"items"`
}

// ItemsCachePath 把名单缓存放在配置文件旁边，跟随 --config / EASY_UNLOCKER_CONFIG。
func ItemsCachePath(configPath string) string {
	resolved := DefaultConfigPath(configPath)
	if resolved == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(resolved), "items.json")
}

// ParseItemList 解析手机封回的名单。
//
// 只认这一个形状，别的一律拒绝：老版本 App 不认识保留名，会把它当普通请求渲染，
// 用户手选一条就真的放出了一个值。调用方拿到错误时绝不能打印或落盘那份载荷。
func ParseItemList(payload []byte) (ItemList, error) {
	var list ItemList
	if err := json.Unmarshal(payload, &list); err != nil {
		return ItemList{}, errors.New("载荷不是条目名名单")
	}
	if list.Version != itemListVersion {
		return ItemList{}, errors.New("名单版本不认识")
	}
	// items 缺失与「空名单」是两回事：前者是别的载荷，后者是库里真没有条目。
	if list.Items == nil {
		return ItemList{}, errors.New("载荷里没有名单")
	}
	for _, entry := range list.Items {
		if strings.TrimSpace(entry.Name) == "" {
			return ItemList{}, errors.New("名单里有空名字")
		}
	}
	return list, nil
}

// LoadItems 读本机缓存的名单；文件不存在时把 os.ErrNotExist 原样透出去。
func LoadItems(path string) (ItemList, error) {
	if strings.TrimSpace(path) == "" {
		return ItemList{}, errors.New("名单缓存路径不能为空")
	}
	material, err := os.ReadFile(path)
	if err != nil {
		return ItemList{}, err
	}
	list, err := ParseItemList(material)
	if err != nil {
		return ItemList{}, errors.New("本机名单缓存无法解析，重跑 easyGet list --refresh")
	}
	return list, nil
}

// SaveItems 原子写名单缓存（0600），与配置文件同目录。
func SaveItems(path string, list ItemList) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("名单缓存路径不能为空")
	}
	list.Version = itemListVersion
	encoded, err := json.Marshal(list)
	if err != nil {
		return errors.New("无法编码名单")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return errors.New("无法创建配置目录")
	}
	return WriteSecret(path, encoded)
}

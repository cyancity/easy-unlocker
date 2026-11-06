package main

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/cyancity/easy-unlocker/internal/boxpayload"
)

// ---------- 待批准 ----------

func (a *App) Pending() ([]pendingView, error) {
	client, err := a.requireClient()
	if err != nil {
		return nil, err
	}
	list, err := client.pending()
	if err != nil {
		return nil, err
	}
	out := make([]pendingView, 0, len(list))
	for _, p := range list {
		if p.State == "waiting" {
			out = append(out, p)
		}
	}
	return out, nil
}

// Approve 把选中条目的密码（或备注）用请求的一次性公钥封回——v2 boxpayload，
// 与手机端同一条密封路径。
func (a *App) Approve(requestID, itemName, field string) error {
	if err := a.requireApprover(); err != nil {
		return err
	}
	client, err := a.requireClient()
	if err != nil {
		return err
	}
	v, err := a.requireVault()
	if err != nil {
		return err
	}
	list, err := client.pending()
	if err != nil {
		return err
	}
	var target *pendingView
	for i := range list {
		if list[i].RequestID == requestID {
			target = &list[i]
		}
	}
	if target == nil {
		return errors.New("请求已不在（过期或被其它设备处理）")
	}
	var value []byte
	if target.Item == "#items" {
		value = a.itemListWire(v)
	} else {
		name := strings.TrimSpace(itemName)
		if name == "" {
			name = target.Item
		}
		item := v.matchItem(name)
		if item == nil {
			return errors.New("库里找不到这条凭据")
		}
		wanted := releaseField(field, target.Field, item)
		if wanted == "note" {
			value = []byte(item.Note)
			if len(value) == 0 {
				return errors.New("这条没有备注可放")
			}
		} else {
			value = []byte(item.Secret)
			if len(value) == 0 {
				return errors.New("这条没有密码——若值在备注里，切到「备注」再批准")
			}
		}
	}
	if strings.TrimSpace(target.SealPublicKey) == "" {
		return errors.New("这次请求没有传输公钥，请用新版 easyGet 重发")
	}
	pub, err := boxpayload.ParsePublic(target.SealPublicKey)
	if err != nil {
		return errors.New("请求公钥无法解析")
	}
	envelope, err := boxpayload.Seal(pub, requestID, value)
	for i := range value {
		value[i] = 0
	}
	if err != nil {
		return errors.New("密封失败")
	}
	return client.decide(requestID, "approve", envelope)
}

// releaseField 决定这次放哪一栏，返回 "note" 或 "secret"。
//
// 优先级：前端选的那栏（用户看得见）→ 请求方指定的栏（CLI 的 --field）→ 旧规矩
// （有密码放密码、没密码放备注）。
//
// 协议里的栏名是 password/note，前端的是 secret/note，这里翻译一次。
func releaseField(chosen, requested string, item *vaultItem) string {
	switch chosen {
	case "note", "secret":
		return chosen
	}
	switch requested {
	case "note":
		return "note"
	case "password":
		return "secret"
	}
	if item.Secret != "" {
		return "secret"
	}
	return "note"
}

func (a *App) Deny(requestID string) error {
	if err := a.requireApprover(); err != nil {
		return err
	}
	client, err := a.requireClient()
	if err != nil {
		return err
	}
	return client.decide(requestID, "deny", "")
}

// itemListWire 对齐 ItemList.kt / cli/items.go：{"v":1,"items":[{name,aliases}]}，只名不值。
func (a *App) itemListWire(v *vault) []byte {
	entries := make([]map[string]any, 0, len(v.plain.Items))
	for _, item := range v.plain.Items {
		entries = append(entries, map[string]any{"name": item.Name, "aliases": item.Aliases})
	}
	raw, _ := json.Marshal(map[string]any{"v": 1, "items": entries})
	return raw
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cyancity/easy-unlocker/cli"
	"github.com/cyancity/easy-unlocker/internal/boxpayload"
	"github.com/cyancity/easy-unlocker/internal/protocol"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App 是 Wails 绑定面：每个导出方法都是 window.go.main.App.<Name>。
// 线程模型：Wails 从 webview 并发调进来，所有可写状态走 a.mu；
// 明文条目只活在内存里，Lock/关机即清。
type App struct {
	ctx context.Context

	mu        sync.Mutex
	cfg       config
	cached    vaultCache
	vault     *vault // nil = 未解锁
	requester *cli.Client
	qrSession *qrPairSession // 进行中的扫码配对，nil = 无
}

func NewApp() *App {
	app := &App{}
	cfg, _ := loadConfig()
	app.cfg = cfg
	app.cached = loadVaultCache()
	if cfg.Token != "" {
		app.requester = &cli.Client{BrokerURL: cfg.BrokerURL, PairingToken: cfg.Token}
	}
	return app
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	go a.watchSessionLock()
}

// ---------- 状态 ----------

type statusView struct {
	Paired       bool   `json:"paired"`
	BrokerURL    string `json:"broker_url"`
	DeviceName   string `json:"device_name"`
	Role         string `json:"role"`
	VaultState   string `json:"vault_state"` // none | locked | unlocked
	VaultItems   int    `json:"vault_items"`
	VaultID      string `json:"vault_id"`
	SyncedAt     string `json:"synced_at"`
	HasLocalBlob bool   `json:"has_local_blob"`
}

func (a *App) Status() statusView {
	a.mu.Lock()
	defer a.mu.Unlock()
	state := "none"
	if a.vault != nil {
		state = "unlocked"
	} else if a.cached.Blob != "" {
		state = "locked"
	}
	view := statusView{
		Paired:       a.cfg.Token != "",
		BrokerURL:    a.cfg.BrokerURL,
		DeviceName:   a.cfg.DeviceName,
		Role:         a.cfg.Role,
		VaultState:   state,
		SyncedAt:     a.cached.UpdatedAt,
		HasLocalBlob: a.cached.Blob != "",
	}
	if a.vault != nil {
		view.VaultItems = len(a.vault.plain.Items)
		view.VaultID = a.vault.plain.VaultID
	}
	return view
}

// ---------- 配对 ----------

// Pair 用手机上的一次性码换设备令牌。role=approver 的码 → 这台机器也是批准端。
// 成功后顺手写 easyGet 配置——同一张令牌两边都能用，用户无感。
func (a *App) Pair(brokerURL, code string) (statusView, error) {
	host, _ := os.Hostname()
	result, err := cli.PairClaimFull(brokerURL, code, host+" (desktop)")
	if err != nil {
		return statusView{}, err
	}
	cfg := config{
		BrokerURL:  strings.TrimRight(strings.TrimSpace(brokerURL), "/"),
		Token:      result.DeviceToken,
		Role:       result.Role,
		DeviceName: result.Name,
	}
	if err := saveConfig(cfg); err != nil {
		return statusView{}, err
	}
	a.mu.Lock()
	a.cfg = cfg
	a.requester = &cli.Client{BrokerURL: cfg.BrokerURL, PairingToken: cfg.Token}
	a.mu.Unlock()
	// 配对完顺手拉一次 vault：批准端拿不到库就只能当请求端用。
	_, _ = a.SyncVault()
	return a.Status(), nil
}

// Unpair 撤销自己这台设备的令牌并清掉本地状态（= 登出这台机器）。
func (a *App) Unpair() error {
	a.mu.Lock()
	client := newBrokerClient(a.cfg.BrokerURL, a.cfg.Token)
	a.mu.Unlock()
	// 找自己的设备 id：撤销端点只认 id，不认令牌。
	devices, err := client.devices()
	var selfID string
	if err == nil {
		for _, d := range devices {
			if d.Current {
				selfID = d.ID
			}
		}
	}
	if selfID != "" {
		// 服务端撤销失败也继续清本地：本地清了这台机器就再也发不出请求。
		_ = client.revokeDevice(selfID)
	}
	a.mu.Lock()
	a.cfg = config{}
	a.requester = nil
	if a.vault != nil {
		a.vault.clear()
	}
	a.vault = nil
	a.cached = vaultCache{}
	a.mu.Unlock()
	return wipeConfig()
}

// ---------- vault 同步与解锁 ----------

// SyncVault 从 broker 拉最新整库密文并写本地缓存。返回是否拿到 blob。
func (a *App) SyncVault() (bool, error) {
	a.mu.Lock()
	client := newBrokerClient(a.cfg.BrokerURL, a.cfg.Token)
	a.mu.Unlock()
	payload, found, err := client.vaultGet()
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	cached := vaultCache{Blob: payload.Blob, Wrap: payload.Wrap, UpdatedAt: payload.UpdatedAt}
	a.mu.Lock()
	a.cached = cached
	a.mu.Unlock()
	saveVaultCache(cached)
	return true, nil
}

// Unlock 先试 password.wrap（解锁密码），不对再试恢复码——同一个入口，
// 用户不用管自己手上是哪把钥匙。
func (a *App) Unlock(secret string) (int, error) {
	a.mu.Lock()
	blob := a.cached.Blob
	wrap := a.cached.Wrap
	updated := a.cached.UpdatedAt
	a.mu.Unlock()
	if blob == "" {
		return 0, errors.New("还没有同步到 vault——在手机上开一次库让它推上来")
	}
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return 0, errors.New("请输入解锁密码或恢复码")
	}
	var key []byte
	if wrap != "" {
		if k, err := unwrapVaultKey(wrap, secret); err == nil {
			key = k
		}
	}
	if key == nil {
		k, err := deriveVaultKeyFromRecovery(blob, secret)
		if err != nil {
			return 0, errors.New("解锁失败：既不是解锁密码也不是恢复码")
		}
		key = k
	}
	plain, err := decryptVault(blob, key)
	if err != nil {
		for i := range key {
			key[i] = 0
		}
		return 0, err
	}
	a.mu.Lock()
	if a.vault != nil {
		a.vault.clear()
	}
	a.vault = &vault{key: key, plain: plain, updated: updated}
	a.mu.Unlock()
	return len(plain.Items), nil
}

func (a *App) Lock() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.vault != nil {
		a.vault.clear()
	}
	a.vault = nil
}

func (a *App) requireVault() (*vault, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.vault == nil {
		return nil, errors.New("库已锁上，请先解锁")
	}
	return a.vault, nil
}

func (a *App) requireClient() (*brokerClient, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cfg.Token == "" {
		return nil, errors.New("还没有配对——先在「设备」页输入手机上生成的配对码")
	}
	return newBrokerClient(a.cfg.BrokerURL, a.cfg.Token), nil
}

func (a *App) requireApprover() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cfg.Role != "approver" {
		return errors.New("这台设备是取凭据角色，不能批准——用「配对桌面批准端」生成的码重配")
	}
	return nil
}

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
		if field == "note" {
			value = []byte(item.Note)
		} else {
			value = []byte(item.Secret)
		}
		if len(value) == 0 {
			return errors.New("这条没有内容可放")
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

// ---------- 条目浏览 ----------

type itemMeta struct {
	Name     string   `json:"name"`
	Aliases  []string `json:"aliases"`
	HasValue bool     `json:"has_value"`
	HasNote  bool     `json:"has_note"`
}

// Items 只给名字与形状，不给值——值只在批准/复制时短暂经过内存。
func (a *App) Items() ([]itemMeta, error) {
	v, err := a.requireVault()
	if err != nil {
		return nil, err
	}
	out := make([]itemMeta, 0, len(v.plain.Items))
	for _, item := range v.plain.Items {
		out = append(out, itemMeta{
			Name:     item.Name,
			Aliases:  item.Aliases,
			HasValue: item.Secret != "",
			HasNote:  item.Note != "",
		})
	}
	return out, nil
}

// CopyItem 把条目值写进系统剪贴板，不落盘也不回前端。
func (a *App) CopyItem(name, field string) error {
	v, err := a.requireVault()
	if err != nil {
		return err
	}
	item := v.matchItem(name)
	if item == nil {
		return errors.New("库里找不到这条凭据")
	}
	value := item.Secret
	if field == "note" {
		value = item.Note
	}
	if value == "" {
		return errors.New("这条没有内容")
	}
	return wailsRuntime.ClipboardSetText(a.ctx, value)
}

// ---------- 发起请求（GUI 也能当 requester 用） ----------

// RequestValue 发起一次 write 请求并长轮询等批准，回来时把明文放进剪贴板。
// 用途：桌面也是取凭据端——CLI 之外给 GUI 一条自己的取密路径。
func (a *App) RequestValue(item, purpose string, ttl int) (string, error) {
	a.mu.Lock()
	req := a.requester
	a.mu.Unlock()
	if req == nil {
		return "", errors.New("还没有配对")
	}
	if ttl <= 0 || ttl > 3600 {
		ttl = 300
	}
	key, err := boxpayload.Generate()
	if err != nil {
		return "", errors.New("无法创建传输密钥")
	}
	defer key.Clear()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ttl)*time.Second)
	defer cancel()
	resp, err := req.Request(ctx, protocol.Request{
		Item:          strings.TrimSpace(item),
		Mode:          protocol.ModeWrite,
		Purpose:       strings.TrimSpace(purpose),
		TTL:           ttl,
		Delivery:      "ephemeral",
		SealPublicKey: key.PublicBase64(),
	})
	if err != nil {
		return "", errors.New("请求失败：" + err.Error())
	}
	if resp.Status != protocol.StatusApproved {
		return "", errors.New("未被批准：" + resp.Status)
	}
	value, err := cli.OpenBoxPayload(resp, key)
	if err != nil {
		return "", err
	}
	defer cli.ClearBytes(value)
	if err := wailsRuntime.ClipboardSetText(a.ctx, string(value)); err != nil {
		return "", errors.New("无法写入剪贴板")
	}
	return "已复制到剪贴板", nil
}

// ---------- 设备管理 ----------

func (a *App) Devices() ([]deviceMeta, error) {
	client, err := a.requireClient()
	if err != nil {
		return nil, err
	}
	return client.devices()
}

func (a *App) RevokeDevice(id string) error {
	if err := a.requireApprover(); err != nil {
		return err
	}
	client, err := a.requireClient()
	if err != nil {
		return err
	}
	return client.revokeDevice(id)
}

func (a *App) RenameDevice(id, name string) error {
	if err := a.requireApprover(); err != nil {
		return err
	}
	client, err := a.requireClient()
	if err != nil {
		return err
	}
	return client.renameDevice(id, name)
}

package main

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/argon2"
)

// vault.eu1 与 password.wrap 的 Go 版解密，逐字对齐 Android 侧
// crypto/VaultCrypto.kt 与 crypto/PasswordWrap.kt：
//   vault key = argon2id(recovery16B, salt, ops, memKiB, p=1) → AES-256-GCM(AAD "easy-unlocker-vault")
//   wrap key  = argon2id(password,    salt, ops, memKiB, p=1) → AES-256-GCM(AAD "easy-unlocker-password-wrap")

var (
	vaultAAD = []byte("easy-unlocker-vault")
	wrapAAD  = []byte("easy-unlocker-password-wrap")
)

type vaultFileJSON struct {
	KDF        string `json:"kdf"`
	Salt       string `json:"salt"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
	Ops        int    `json:"ops"`
	MemKiB     int    `json:"memKiB"`
}

type wrapFileJSON struct {
	KDF    string `json:"kdf"`
	Salt   string `json:"salt"`
	IV     string `json:"iv"`
	CT     string `json:"ct"`
	Ops    int    `json:"ops"`
	MemKiB int    `json:"memKiB"`
}

// vaultItem 是明文条目，与 Android VaultItem 同形状。
type vaultItem struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
	Secret  string   `json:"secret"`
	Note    string   `json:"note"`
}

type vaultPlaintext struct {
	Items   []vaultItem `json:"items"`
	VaultID string      `json:"vault_id"`
}

type vault struct {
	key     []byte
	plain   vaultPlaintext
	updated string // 密文的 updated_at，展示用
}

func (v *vault) clear() {
	for i := range v.key {
		v.key[i] = 0
	}
	v.key = nil
	v.plain = vaultPlaintext{}
}

// matchItem 按名字/别名大小写不敏感匹配，命中多于一条视为不匹配（对齐 Android matchOrNull）。
func (v *vault) matchItem(name string) *vaultItem {
	key := strings.ToLower(strings.TrimSpace(name))
	var hit *vaultItem
	for i := range v.plain.Items {
		it := &v.plain.Items[i]
		if strings.ToLower(it.Name) == key {
			if hit != nil {
				return nil
			}
			hit = it
			continue
		}
		for _, a := range it.Aliases {
			if strings.ToLower(a) == key {
				if hit != nil {
					return nil
				}
				hit = it
			}
		}
	}
	return hit
}

// normalizeRecovery 对齐 VaultCrypto.normalizeRecovery：去短横线/空格、小写、32 位 hex。
func normalizeRecovery(code string) ([]byte, error) {
	cleaned := strings.NewReplacer("-", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(code)))
	raw, err := hex.DecodeString(cleaned)
	if err != nil || len(raw) != 16 {
		return nil, errors.New("恢复码长度不对")
	}
	return raw, nil
}

func argonKey(secret []byte, salt []byte, ops, memKiB int) []byte {
	return argon2.IDKey(secret, salt, uint32(ops), uint32(memKiB), 1, 32)
}

func openGCM(key, nonce, ciphertext, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.New("无法初始化加密器")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.New("无法初始化认证器")
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, errors.New("解密失败")
	}
	return plain, nil
}

func b64d(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(strings.TrimSpace(s))
}

// deriveVaultKeyFromRecovery：vault.eu1 的 salt + 恢复码 → vault key。
func deriveVaultKeyFromRecovery(blob string, recoveryCode string) ([]byte, error) {
	var vf vaultFileJSON
	if err := json.Unmarshal([]byte(blob), &vf); err != nil {
		return nil, errors.New("vault 密文格式不对")
	}
	recovery, err := normalizeRecovery(recoveryCode)
	if err != nil {
		return nil, err
	}
	defer func(b []byte) {
		for i := range b {
			b[i] = 0
		}
	}(recovery)
	salt, err := b64d(vf.Salt)
	if err != nil {
		return nil, errors.New("vault salt 无法解析")
	}
	ops, mem := vf.Ops, vf.MemKiB
	if ops <= 0 {
		ops = 3
	}
	if mem <= 0 {
		mem = 64 * 1024
	}
	return argonKey(recovery, salt, ops, mem), nil
}

// unwrapVaultKey：password.wrap + 解锁密码 → vault key。
func unwrapVaultKey(wrapJSON, password string) ([]byte, error) {
	var wf wrapFileJSON
	if err := json.Unmarshal([]byte(wrapJSON), &wf); err != nil {
		return nil, errors.New("密码包裹格式不对")
	}
	salt, err := b64d(wf.Salt)
	if err != nil {
		return nil, errors.New("密码包裹无法解析")
	}
	iv, err := b64d(wf.IV)
	if err != nil {
		return nil, errors.New("密码包裹无法解析")
	}
	ct, err := b64d(wf.CT)
	if err != nil {
		return nil, errors.New("密码包裹无法解析")
	}
	ops, mem := wf.Ops, wf.MemKiB
	if ops <= 0 {
		ops = 3
	}
	if mem <= 0 {
		mem = 64 * 1024
	}
	pw := []byte(password)
	derived := argonKey(pw, salt, ops, mem)
	for i := range pw {
		pw[i] = 0
	}
	key, err := openGCM(derived, iv, ct, wrapAAD)
	for i := range derived {
		derived[i] = 0
	}
	if err != nil {
		return nil, errors.New("密码不对")
	}
	return key, nil
}

// decryptVault：vault key → 明文条目。
func decryptVault(blob string, key []byte) (vaultPlaintext, error) {
	var vf vaultFileJSON
	if err := json.Unmarshal([]byte(blob), &vf); err != nil {
		return vaultPlaintext{}, errors.New("vault 密文格式不对")
	}
	nonce, err := b64d(vf.Nonce)
	if err != nil {
		return vaultPlaintext{}, errors.New("vault 密文无法解析")
	}
	ct, err := b64d(vf.Ciphertext)
	if err != nil {
		return vaultPlaintext{}, errors.New("vault 密文无法解析")
	}
	plain, err := openGCM(key, nonce, ct, vaultAAD)
	if err != nil {
		return vaultPlaintext{}, errors.New("密钥不对，无法解开 vault")
	}
	var decoded vaultPlaintext
	if err := json.Unmarshal(plain, &decoded); err != nil {
		return vaultPlaintext{}, errors.New("vault 明文格式不对")
	}
	return decoded, nil
}

// vaultCachePath 本地缓存最近一次同步的 vault 密文：下次启动不用等网络就能解锁。
func vaultCachePath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "vault-cache.json"), nil
}

type vaultCache struct {
	Blob      string `json:"blob"`
	Wrap      string `json:"wrap"`
	UpdatedAt string `json:"updated_at"`
}

func loadVaultCache() vaultCache {
	path, err := vaultCachePath()
	if err != nil {
		return vaultCache{}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return vaultCache{}
	}
	var cached vaultCache
	if json.Unmarshal(raw, &cached) != nil {
		return vaultCache{}
	}
	return cached
}

func saveVaultCache(cached vaultCache) {
	path, err := vaultCachePath()
	if err != nil {
		return
	}
	raw, err := json.Marshal(cached)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, raw, 0o600)
}

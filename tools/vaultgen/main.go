// vaultgen —— 造一个模拟器用的 vault.eu1。
//
// 格式逐字对齐 android/crypto/VaultCrypto.kt 与 ios/Crypto/VaultCrypto.swift：
// argon2id(recovery16B, salt, ops=3, mem=64MiB, p=1) → AES-256-GCM(AAD "easy-unlocker-vault")。
//
// 用法：go run ./tools/vaultgen <recovery-code> <outfile>
package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/argon2"
)

type vaultFileJSON struct {
	KDF        string `json:"kdf"`
	Salt       string `json:"salt"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
	Ops        int    `json:"ops"`
	MemKiB     int    `json:"memKiB"`
}

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

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "用法：go run vaultgen.go <recovery-code> <outfile>")
		os.Exit(1)
	}
	recovery := normalizeRecovery(os.Args[1])

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		panic(err)
	}
	key := argon2.IDKey(recovery, salt, 3, 64*1024, 1, 32)

	plaintext := vaultPlaintext{
		VaultID: "11111111-2222-3333-4444-555555555555",
		Items: []vaultItem{{
			ID:      "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
			Name:    "my-token",
			Aliases: []string{},
			Secret:  "short-password-value",
			Note:    "long-note-value-in-note-field",
		}},
	}
	raw, err := json.Marshal(plaintext)
	if err != nil {
		panic(err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		panic(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		panic(err)
	}
	ciphertext := gcm.Seal(nil, nonce, raw, []byte("easy-unlocker-vault"))

	blob, err := json.Marshal(vaultFileJSON{
		KDF:        "argon2id",
		Salt:       base64.StdEncoding.EncodeToString(salt),
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
		Ops:        3,
		MemKiB:     64 * 1024,
	})
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(os.Args[2], blob, 0o600); err != nil {
		panic(err)
	}
	fmt.Printf("写入 %s（恢复码 %s）\n", os.Args[2], os.Args[1])
}

// normalizeRecovery 对齐 VaultCrypto.normalizeRecovery：去短横线/空格、小写、32 位 hex。
func normalizeRecovery(code string) []byte {
	cleaned := strings.NewReplacer("-", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(code)))
	raw, err := hex.DecodeString(cleaned)
	if err != nil || len(raw) != 16 {
		fmt.Fprintln(os.Stderr, "恢复码长度不对")
		os.Exit(1)
	}
	return raw
}

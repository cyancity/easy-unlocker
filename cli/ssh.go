package cli

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type SSHIdentity struct {
	Signer       ssh.Signer
	GeneratedKey ed25519.PrivateKey
	GeneratedPEM []byte
	IdentityPath string
}

// 手机签发器只出 ssh-ed25519 证书：复用的身份必须是 ed25519。
// 自动扫到别的算法（如 id_rsa）就跳过，改生成一次性 ed25519；
// 显式 --identity 给非 ed25519 直接报错，免得请求发到手机才被拒。
func PrepareSSHIdentity(identityPath, identityTo, certPath string) (*SSHIdentity, error) {
	if identityPath != "" {
		identity, err := loadSSHIdentity(identityPath)
		if err != nil {
			return nil, err
		}
		return requireEd25519(identity)
	}
	if identityTo == "" {
		identityTo = defaultGeneratedIdentityPath(certPath)
	}
	if identityTo != "" {
		if _, err := os.Stat(identityTo); err == nil {
			identity, err := loadSSHIdentity(identityTo)
			if err != nil {
				return nil, err
			}
			return requireEd25519(identity)
		}
	}
	for _, candidate := range defaultIdentityCandidates() {
		if _, err := os.Stat(candidate); err == nil {
			if identity, err := loadSSHIdentity(candidate); err == nil && identity.isEd25519() {
				return identity, nil
			}
		}
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, errors.New("无法生成临时 SSH 身份")
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		ClearBytes(privateKey)
		return nil, errors.New("无法初始化临时 SSH 身份")
	}
	block, err := ssh.MarshalPrivateKey(privateKey, "easy-unlocker ephemeral identity")
	if err != nil {
		ClearBytes(privateKey)
		return nil, errors.New("无法编码临时 SSH 身份")
	}
	return &SSHIdentity{
		Signer:       signer,
		GeneratedKey: privateKey,
		GeneratedPEM: pemEncode(block),
		IdentityPath: identityTo,
	}, nil
}

func loadSSHIdentity(path string) (*SSHIdentity, error) {
	material, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("无法读取 SSH 身份密钥")
	}
	signer, err := ssh.ParsePrivateKey(material)
	ClearBytes(material)
	if err != nil {
		return nil, errors.New("无法解析 SSH 身份密钥")
	}
	return &SSHIdentity{Signer: signer, IdentityPath: path}, nil
}

func (identity *SSHIdentity) isEd25519() bool {
	return identity.Signer.PublicKey().Type() == ssh.KeyAlgoED25519
}

func requireEd25519(identity *SSHIdentity) (*SSHIdentity, error) {
	if !identity.isEd25519() {
		return nil, errors.New("SSH 身份必须是 ed25519 密钥（手机签发器只支持 ed25519 证书）")
	}
	return identity, nil
}

func (identity *SSHIdentity) PublicKeyLine() string {
	return string(ssh.MarshalAuthorizedKey(identity.Signer.PublicKey()))
}

func (identity *SSHIdentity) SaveGenerated() error {
	if len(identity.GeneratedPEM) == 0 || identity.IdentityPath == "" {
		return nil
	}
	if err := WriteProtectedFile(identity.IdentityPath, identity.GeneratedPEM, 0o600); err != nil {
		return err
	}
	return nil
}

func (identity *SSHIdentity) Clear() {
	ClearBytes(identity.GeneratedKey)
	ClearBytes(identity.GeneratedPEM)
}

// VerifyCertificate 校验手机签回的证书确实是这次请求要的那张：
// 用户证书、现在可用、没签出比请求更长的有效期、绑定的就是本地身份、principal 一致。
// 有效期放宽 60 秒吸收手机与本机的时钟差。
func VerifyCertificate(certBytes []byte, identity *SSHIdentity, certTTL int, principal string) error {
	key, _, _, rest, err := ssh.ParseAuthorizedKey(certBytes)
	if err != nil || len(strings.TrimSpace(string(rest))) != 0 {
		return errors.New("手机返回了无效 SSH 证书")
	}
	certificate, ok := key.(*ssh.Certificate)
	if !ok || certificate.Key == nil || certificate.CertType != ssh.UserCert || certificate.Signature == nil {
		return errors.New("手机返回的不是 SSH 用户证书")
	}
	now := uint64(time.Now().Unix())
	if certificate.ValidAfter > now+30 || certificate.ValidBefore <= now {
		return errors.New("手机返回的 SSH 证书有效期异常")
	}
	if certTTL > 0 && certificate.ValidBefore-now > uint64(certTTL)+60 {
		return errors.New("SSH 证书的有效期超过了请求值")
	}
	if !bytes.Equal(certificate.Key.Marshal(), identity.Signer.PublicKey().Marshal()) {
		return errors.New("SSH 证书与本地身份不匹配")
	}
	if len(certificate.ValidPrincipals) != 1 || certificate.ValidPrincipals[0] != principal {
		return errors.New("SSH 证书的 principal 与请求不符")
	}
	return nil
}

func WriteProtectedFile(path string, value []byte, mode os.FileMode) error {
	if path == "" {
		return errors.New("文件路径不能为空")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return errors.New("无法创建文件目录")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return errors.New("目标文件已存在或无法创建")
	}
	if _, err := file.Write(value); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return errors.New("无法写入文件")
	}
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return errors.New("无法保护文件")
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return errors.New("无法关闭文件")
	}
	return nil
}

// WritePublicFile replaces a public certificate atomically. Certificates are
// safe to replace between approvals; using a separate helper keeps private
// identity files create-once and prevents accidental overwrites.
func WritePublicFile(path string, value []byte) error {
	if path == "" {
		return errors.New("文件路径不能为空")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return errors.New("无法创建证书目录")
	}
	file, err := os.CreateTemp(dir, ".easy-unlocker-cert-*")
	if err != nil {
		return errors.New("无法创建证书临时文件")
	}
	temporary := file.Name()
	removeTemporary := true
	defer func() {
		_ = file.Close()
		if removeTemporary {
			_ = os.Remove(temporary)
		}
	}()
	if err := file.Chmod(0o644); err != nil {
		return errors.New("无法保护证书文件")
	}
	if _, err := file.Write(value); err != nil {
		return errors.New("无法写入证书文件")
	}
	if err := file.Sync(); err != nil {
		return errors.New("无法持久化证书文件")
	}
	if err := file.Close(); err != nil {
		return errors.New("无法关闭证书文件")
	}
	if err := os.Rename(temporary, path); err != nil {
		return errors.New("无法替换证书文件")
	}
	removeTemporary = false
	return nil
}

// defaultGeneratedIdentityPath 从证书路径推身份路径。证书约定名是
// <name>-cert.pub：剥掉整个 "-cert.pub" 得到 <name>，这样 `ssh -i <name>`
// 会自动加载 <name>-cert.pub（OpenSSH 的证书自动加载规则）。别的形状就只剥扩展名。
func defaultGeneratedIdentityPath(certPath string) string {
	if certPath == "" {
		return ""
	}
	if identityPath := strings.TrimSuffix(certPath, "-cert.pub"); identityPath != certPath {
		return identityPath
	}
	identityPath := strings.TrimSuffix(certPath, filepath.Ext(certPath))
	if identityPath == certPath || identityPath == "." || identityPath == "" {
		return certPath + ".key"
	}
	return identityPath
}

func defaultIdentityCandidates() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, ".ssh", "id_ed25519"),
		filepath.Join(home, ".ssh", "id_rsa"),
	}
}

func pemEncode(block *pem.Block) []byte {
	return pem.EncodeToMemory(block)
}

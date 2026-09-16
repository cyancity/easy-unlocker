package cli

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestGeneratedSSHIdentityStaysLocalAndProtected(t *testing.T) {
	directory := t.TempDir()
	// 让「找现成密钥」的候选路径（$HOME/.ssh/id_ed25519 等）落进临时目录，
	// 否则作者机器上有自己的 SSH key 时，这里会复用现成身份而不是生成临时的。
	t.Setenv("HOME", directory)
	certPath := filepath.Join(directory, "identity-cert.pub")
	identity, err := PrepareSSHIdentity("", "", certPath)
	if err != nil {
		t.Fatal(err)
	}
	defer identity.Clear()
	if identity.PublicKeyLine() == "" || len(identity.GeneratedPEM) == 0 {
		t.Fatal("did not create ephemeral SSH identity")
	}
	parsed, _, _, rest, err := ssh.ParseAuthorizedKey([]byte(identity.PublicKeyLine()))
	if err != nil || parsed == nil || len(rest) != 0 {
		t.Fatalf("invalid generated public key: err=%v", err)
	}
	if err := identity.SaveGenerated(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(identity.IdentityPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("generated private key mode=%o", info.Mode().Perm())
	}
	material, err := os.ReadFile(identity.IdentityPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ssh.ParsePrivateKey(material); err != nil {
		t.Fatal("generated private key cannot be parsed")
	}
	ClearBytes(material)
	second, err := PrepareSSHIdentity("", "", certPath)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Clear()
	if second.PublicKeyLine() != identity.PublicKeyLine() || len(second.GeneratedPEM) != 0 {
		t.Fatal("existing generated identity was not reused")
	}
}

func TestPublicCertificateCanBeRefreshed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "certificate.pub")
	if err := WritePublicFile(path, []byte("first\n")); err != nil {
		t.Fatal(err)
	}
	if err := WritePublicFile(path, []byte("second\n")); err != nil {
		t.Fatal(err)
	}
	value, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(value) != "second\n" {
		t.Fatalf("certificate was not refreshed")
	}
	if mode := fileMode(t, path).Perm(); mode != 0o644 {
		t.Fatalf("certificate mode=%o", mode)
	}
}

// phoneSignsCert 扮演手机端签发：测试里用它代替 App 出证书，形状与 App 的
// SshCert.signUser 输出必须一致（字段对齐 PROTOCOL.certkeys）。
func phoneSignsCert(t *testing.T, ca ssh.Signer, identity *SSHIdentity, requestID, principal string, certTTL int) []byte {
	t.Helper()
	publicKey, _, _, rest, err := ssh.ParseAuthorizedKey([]byte(identity.PublicKeyLine()))
	if err != nil || len(rest) != 0 {
		t.Fatalf("identity public key does not parse: %v", err)
	}
	now := time.Now()
	certificate := &ssh.Certificate{
		Nonce:           make([]byte, 32),
		Key:             publicKey,
		CertType:        ssh.UserCert,
		KeyId:           "easy-unlocker/" + requestID,
		ValidPrincipals: []string{principal},
		ValidAfter:      uint64(now.Add(-30 * time.Second).Unix()),
		ValidBefore:     uint64(now.Add(time.Duration(certTTL) * time.Second).Unix()),
		Permissions: ssh.Permissions{Extensions: map[string]string{
			"permit-X11-forwarding":   "",
			"permit-agent-forwarding": "",
			"permit-port-forwarding":  "",
			"permit-pty":              "",
			"permit-user-rc":          "",
		}},
	}
	if _, err := rand.Read(certificate.Nonce); err != nil {
		t.Fatal(err)
	}
	if err := certificate.SignCert(rand.Reader, ca); err != nil {
		t.Fatal(err)
	}
	return ssh.MarshalAuthorizedKey(certificate)
}

func TestVerifyCertificateBindsToLocalIdentity(t *testing.T) {
	directory := t.TempDir()
	_, caKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := ssh.NewSignerFromKey(caKey)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := PrepareSSHIdentity("", "", filepath.Join(directory, "cert.pub"))
	if err != nil {
		t.Fatal(err)
	}
	defer identity.Clear()
	certificate := phoneSignsCert(t, ca, identity, "request-123", "ubuntu", 300)
	if err := VerifyCertificate(certificate, identity, 300, "ubuntu"); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyCertificateRejectsWrongPrincipalAndLongerTTL(t *testing.T) {
	directory := t.TempDir()
	_, caKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := ssh.NewSignerFromKey(caKey)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := PrepareSSHIdentity("", "", filepath.Join(directory, "cert.pub"))
	if err != nil {
		t.Fatal(err)
	}
	defer identity.Clear()
	if err := VerifyCertificate(phoneSignsCert(t, ca, identity, "r1", "root", 300), identity, 300, "ubuntu"); err == nil {
		t.Fatal("principal 不符的证书被接受了")
	}
	if err := VerifyCertificate(phoneSignsCert(t, ca, identity, "r2", "ubuntu", 3600), identity, 300, "ubuntu"); err == nil {
		t.Fatal("比请求更长的证书有效期被接受了")
	}
}

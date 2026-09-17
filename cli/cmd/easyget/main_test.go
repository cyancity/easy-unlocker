package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cyancity/easy-unlocker/broker"
	"github.com/cyancity/easy-unlocker/cli"
	"github.com/cyancity/easy-unlocker/internal/boxpayload"
	"github.com/cyancity/easy-unlocker/internal/protocol"
	"golang.org/x/crypto/ssh"
)

func TestResolveReleaseTarget(t *testing.T) {
	cases := []struct {
		name         string
		writeTo      string
		exec         string
		wantTarget   string
		wantDelivery string
		wantErr      bool
	}{
		{"落盘", "/tmp/x.env", "", "/tmp/x.env", protocol.DeliveryFile, false},
		{"不落盘", "", "python x.py", "", protocol.DeliveryEphemeral, false},
		{"空白路径也算给了", "  ", "true", "", protocol.DeliveryEphemeral, false},
		{"两个都给", "/tmp/x", "true", "", "", true},
		{"一个都不给", "", "", "", "", true},
		{"只有空白", "  ", "  ", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target, delivery, err := resolveReleaseTarget(tc.writeTo, tc.exec)
			if tc.wantErr {
				if err == nil {
					t.Fatal("want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if target != tc.wantTarget || delivery != tc.wantDelivery {
				t.Fatalf("got (%q, %q), want (%q, %q)", target, delivery, tc.wantTarget, tc.wantDelivery)
			}
		})
	}
}

type listNotifier struct{ ch chan broker.Notification }

func (n *listNotifier) Notify(_ context.Context, notification broker.Notification) error {
	n.ch <- notification
	return nil
}

type listAudit struct{}

func (listAudit) Record(broker.AuditEvent) error { return nil }

// listBroker 起一个真 Broker：Store 里放一条真凭据，证明名单这条路根本不碰它。
func listBroker(t *testing.T) (*httptest.Server, chan broker.Notification) {
	t.Helper()
	notifier := &listNotifier{ch: make(chan broker.Notification, 1)}
	server, err := broker.NewServer(broker.ServerConfig{
		PairingToken: "pairing-token",
		AdminToken:   "admin-token",
		Store:        broker.NewMapStore(map[string][]byte{"OPENAI_API_KEY": []byte("store-secret-must-not-leak\n")}),
		Notifier:     notifier,
		Audit:        listAudit{},
		MaxTTL:       time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	return httpServer, notifier.ch
}

// pendingSealKey 就是手机替身要拿的东西：CLI 为这次请求现生成的公钥。
func pendingSealKey(t *testing.T, baseURL, requestID string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		request, err := http.NewRequest(http.MethodGet, baseURL+"/v1/admin/pending", nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("X-Admin-Token", "Bearer admin-token")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		material, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("admin/pending status=%d body=%s", response.StatusCode, material)
		}
		var views []broker.PendingView
		if err := json.Unmarshal(material, &views); err != nil {
			t.Fatal(err)
		}
		for _, view := range views {
			if view.RequestID == requestID {
				if view.Item != cli.ReservedListItem {
					t.Fatalf("item=%q, want the reserved name", view.Item)
				}
				return view.SealPublicKey
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("pending 里等不到这条请求")
	return ""
}

func sealAsPhone(t *testing.T, sealKey, requestID, plaintext string) string {
	t.Helper()
	public, err := boxpayload.ParsePublic(sealKey)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := boxpayload.Seal(public, requestID, []byte(plaintext))
	if err != nil {
		t.Fatal(err)
	}
	return envelope
}

func approveWith(t *testing.T, baseURL, requestID, signature, envelope string) {
	t.Helper()
	body, err := json.Marshal(protocol.Decision{Decision: "approve", Sig: signature, Payload: envelope})
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(baseURL+"/v1/decision/"+requestID, "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("decision status=%d", response.StatusCode)
	}
}

// captureStdout 把 stdout 换成临时文件：要断言「那条凭据没有被打印出来」。
func captureStdout(t *testing.T) func() string {
	t.Helper()
	original := os.Stdout
	file, err := os.CreateTemp(t.TempDir(), "stdout-*")
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = file
	t.Cleanup(func() {
		os.Stdout = original
		_ = file.Close()
	})
	return func() string {
		material, err := os.ReadFile(file.Name())
		if err != nil {
			t.Fatal(err)
		}
		return string(material)
	}
}

func TestListRefreshCachesTheListAndRefusesAnythingElse(t *testing.T) {
	const sneaky = "sk-live-should-never-be-written"
	cases := []struct {
		name    string
		phone   func(t *testing.T, sealKey, requestID string) string
		wantErr bool
	}{
		{
			name: "手机封回一份名单",
			phone: func(t *testing.T, sealKey, requestID string) string {
				return sealAsPhone(t, sealKey, requestID, `{"v":1,"items":[{"name":"openai","aliases":["OPENAI_API_KEY"]}]}`)
			},
		},
		{
			// 老版本 App 不认识保留名：用户手选一条、点了批准，这里收到的就是一条真实凭据。
			name: "老 App 手选了一条真实凭据",
			phone: func(t *testing.T, sealKey, requestID string) string {
				return sealAsPhone(t, sealKey, requestID, sneaky)
			},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			httpServer, notifications := listBroker(t)
			cachePath := cli.ItemsCachePath(filepath.Join(t.TempDir(), "easy-unlocker", "config"))
			settings := cli.Settings{BrokerURL: httpServer.URL, PairingToken: "pairing-token", Requester: "agent@test"}

			printed := captureStdout(t)
			flags := newCommonFlags("easyGet list", settings)
			flags.ttl = 10 // 测试 Broker 的 MaxTTL 是一分钟
			done := make(chan error, 1)
			go func() {
				done <- runListRefresh(settings, cachePath, flags)
			}()
			var notification broker.Notification
			select {
			case notification = <-notifications:
			case <-time.After(10 * time.Second):
				t.Fatal("Broker 没有把这条请求推出来")
			}
			approveWith(t, httpServer.URL, notification.RequestID, notification.ApproveSig,
				tc.phone(t, pendingSealKey(t, httpServer.URL, notification.RequestID), notification.RequestID))
			err := <-done
			out := printed()

			if strings.Contains(out, sneaky) || strings.Contains(out, "store-secret-must-not-leak") {
				t.Fatalf("凭据被打印到了 stdout: %q", out)
			}
			if tc.wantErr {
				if err == nil {
					t.Fatal("want error, got nil")
				}
				if !strings.Contains(err.Error(), "没有落盘也没有打印") {
					t.Fatalf("错误没说清落地情况: %v", err)
				}
				if _, statErr := os.Stat(cachePath); !os.IsNotExist(statErr) {
					t.Fatalf("载荷不是名单却写了缓存: %v", statErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			cached, err := cli.LoadItems(cachePath)
			if err != nil {
				t.Fatal(err)
			}
			if len(cached.Items) != 1 || cached.Items[0].Name != "openai" || len(cached.Items[0].Aliases) != 1 {
				t.Fatalf("缓存内容不对: %+v", cached.Items)
			}
			if cached.FetchedAt == "" {
				t.Fatal("缓存没有记下取回时间")
			}
			if !strings.Contains(out, "openai") {
				t.Fatalf("名单没打印出来: %q", out)
			}
		})
	}
}

// pendingViewFor 拿到整条 pending 视图：sign 请求要 public_key/ssh_user/cert_ttl 都在，
// 否则手机拿不到要签的东西。
func pendingViewFor(t *testing.T, baseURL, requestID string) broker.PendingView {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		request, err := http.NewRequest(http.MethodGet, baseURL+"/v1/admin/pending", nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("X-Admin-Token", "Bearer admin-token")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		material, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("admin/pending status=%d body=%s", response.StatusCode, material)
		}
		var views []broker.PendingView
		if err := json.Unmarshal(material, &views); err != nil {
			t.Fatal(err)
		}
		for _, view := range views {
			if view.RequestID == requestID {
				return view
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("pending 里等不到这条请求")
	return broker.PendingView{}
}

// signAsPhone 扮演手机 App：解析请求里的公钥，用测试 CA 签一张证书。
// 字段与 App 的 SshCert.signUser 输出必须一致。
func signAsPhone(t *testing.T, view broker.PendingView) string {
	t.Helper()
	_, caKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := ssh.NewSignerFromKey(caKey)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, _, _, rest, err := ssh.ParseAuthorizedKey([]byte(view.PublicKey))
	if err != nil || len(rest) != 0 {
		t.Fatalf("request public_key does not parse: %v", err)
	}
	now := time.Now()
	certificate := &ssh.Certificate{
		Nonce:           make([]byte, 32),
		Key:             publicKey,
		CertType:        ssh.UserCert,
		KeyId:           "easy-unlocker/" + view.RequestID,
		ValidPrincipals: []string{view.SSHUser},
		ValidAfter:      uint64(now.Add(-30 * time.Second).Unix()),
		ValidBefore:     uint64(now.Add(time.Duration(view.CertTTL) * time.Second).Unix()),
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
	return string(ssh.MarshalAuthorizedKey(certificate))
}

// 端到端：easyGet ssh → Broker → 手机替身签发 → 证书与身份落盘。
// 顺带验证 sign 请求的三字段经 pendingView 透传到了手机侧。
func TestSSHRequestGetsPhoneSignedCertificate(t *testing.T) {
	httpServer, notifications := listBroker(t)
	directory := t.TempDir()
	t.Setenv("HOME", directory)
	settings := cli.Settings{BrokerURL: httpServer.URL, PairingToken: "pairing-token", Requester: "agent@test"}

	done := make(chan error, 1)
	go func() {
		done <- runSSH(settings, "my-ca", []string{"--for", "deploy", "--ssh-user", "deployer", "--cert-ttl", "600", "--ttl", "10"})
	}()
	var notification broker.Notification
	select {
	case notification = <-notifications:
	case <-time.After(10 * time.Second):
		t.Fatal("Broker 没有把这条请求推出来")
	}
	view := pendingViewFor(t, httpServer.URL, notification.RequestID)
	if view.Mode != protocol.ModeSign || view.PublicKey == "" || view.SSHUser != "deployer" || view.CertTTL != 600 {
		t.Fatalf("pending 视图缺 sign 字段: %+v", view)
	}
	if view.SealPublicKey == "" {
		t.Fatal("sign 请求必须带 seal_public_key")
	}
	certLine := signAsPhone(t, view)
	approveWith(t, httpServer.URL, view.RequestID, notification.ApproveSig,
		sealAsPhone(t, view.SealPublicKey, view.RequestID, certLine))
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	certPath := filepath.Join(directory, ".ssh", "easy-unlocker-my-ca-cert.pub")
	material, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("证书没有落盘: %v", err)
	}
	parsed, _, _, rest, err := ssh.ParseAuthorizedKey(material)
	if err != nil || len(strings.TrimSpace(string(rest))) != 0 {
		t.Fatalf("落盘的不是合法 authorized_keys 行: %v", err)
	}
	cert, ok := parsed.(*ssh.Certificate)
	if !ok || cert.CertType != ssh.UserCert {
		t.Fatal("落盘的不是 SSH 用户证书")
	}
	if len(cert.ValidPrincipals) != 1 || cert.ValidPrincipals[0] != "deployer" {
		t.Fatalf("principal 不对: %v", cert.ValidPrincipals)
	}
	identityPath := filepath.Join(directory, ".ssh", "easy-unlocker-my-ca")
	if strings.HasSuffix(identityPath, "-cert") {
		t.Fatalf("身份名不该以 -cert 结尾（否则 ssh -i 找不到自动加载的证书名）: %s", identityPath)
	}
	info, err := os.Stat(identityPath)
	if err != nil {
		t.Fatalf("临时身份没有落盘: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("临时身份 mode=%o", info.Mode().Perm())
	}
}

// pairApprover 用全局配对令牌登记一台审批端（手机替身），返回它的设备令牌。
func pairApprover(t *testing.T, baseURL string) string {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, baseURL+"/v1/device/pair", strings.NewReader(`{"name":"phone"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer pairing-token")
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result struct {
		DeviceToken string `json:"device_token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || result.DeviceToken == "" {
		t.Fatalf("pair status=%d token=%q", response.StatusCode, result.DeviceToken)
	}
	return result.DeviceToken
}

func newPairCode(t *testing.T, baseURL, approverToken string) string {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, baseURL+"/v1/device/pair-code", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+approverToken)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || result.Code == "" {
		t.Fatalf("pair-code status=%d code=%q", response.StatusCode, result.Code)
	}
	return result.Code
}

// requesterName 从设备表里取请求端的名字——手机「设备」页看到的就是它。
func requesterName(t *testing.T, baseURL, approverToken string) string {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, baseURL+"/v1/device/devices", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+approverToken)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result struct {
		Devices []struct {
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"devices"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	for _, device := range result.Devices {
		if device.Role == "requester" {
			return device.Name
		}
	}
	t.Fatal("设备表里没有请求端")
	return ""
}

func TestPairNamesTheDevice(t *testing.T) {
	cases := []struct {
		name      string
		requester string
		args      []string
		want      string
	}{
		{"不给名字就用 user@host", "tester@host", nil, "tester@host"},
		{"显式 --name 覆盖", "tester@host", []string{"--name", "wsl-arch"}, "wsl-arch"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := listBroker(t)
			approver := pairApprover(t, server.URL)
			code := newPairCode(t, server.URL, approver)
			args := append([]string{"--code", code}, tc.args...)

			stdout := captureStdout(t)
			settings := cli.Settings{BrokerURL: server.URL, Requester: tc.requester}
			if err := runPair(settings, filepath.Join(t.TempDir(), "config"), args); err != nil {
				t.Fatal(err)
			}
			if got := requesterName(t, server.URL, approver); got != tc.want {
				t.Fatalf("设备名=%q, want %q", got, tc.want)
			}
			if printed := stdout(); !strings.Contains(printed, tc.want) {
				t.Fatalf("stdout 没打印设备名 %q: %s", tc.want, printed)
			}
		})
	}
}

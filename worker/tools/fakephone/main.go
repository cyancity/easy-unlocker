// fakephone 是 spike 用的「假手机」：本地 wrangler dev 时替代 App 完成密封 + 批准/拒绝。
//
// 它用仓库里同一份 internal/boxpayload 生成信封，所以 easyGet CLI 能真的用它的临时私钥解开 ——
// 这条链路（CLI → Worker → 假手机 → CLI）就是 Phase A 的验收，不需要 CF 账号。
//
//	go run ./worker/tools/fakephone -pair -pairing <pairing-token>          # 换一个设备令牌
//	go run ./worker/tools/fakephone -token <device-token> -item X -secret sk-test
//	go run ./worker/tools/fakephone -token <device-token> -item CA -ca /tmp/ca  # sign 请求：签证书
package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cyancity/easy-unlocker/internal/boxpayload"
	"golang.org/x/crypto/ssh"
)

type pendingView struct {
	RequestID     string `json:"request_id"`
	Item          string `json:"item"`
	Mode          string `json:"mode"`
	State         string `json:"state"`
	SealPublicKey string `json:"seal_public_key"`
	PublicKey     string `json:"public_key"`
	SSHUser       string `json:"ssh_user"`
	CertTTL       int    `json:"cert_ttl"`
}

func main() {
	broker := flag.String("broker", "http://127.0.0.1:8787", "Broker 地址")
	token := flag.String("token", "", "设备令牌")
	pair := flag.Bool("pair", false, "只做配对：用 pairing token 换一个设备令牌")
	pairing := flag.String("pairing", "", "pairing token（配 pair 用）")
	vault := flag.String("vault", "", "配对时声明的 vault_id（租户）；留空归 default")
	item := flag.String("item", "", "只处理这个请求名；留空则取第一条待批准")
	secret := flag.String("secret", "", "批准时回传的明文（spike 用假值）")
	ca := flag.String("ca", "", "sign 请求用这个 OpenSSH 私钥当 CA 签证书")
	deny := flag.Bool("deny", false, "改成拒绝")
	wait := flag.Duration("wait", 30*time.Second, "最多等多久出现待批准")
	flag.Parse()

	if *pair {
		if strings.TrimSpace(*pairing) == "" {
			fail("配对需要 -pairing")
		}
		raw, status, err := postRaw(*broker, "/v1/device/pair", *pairing, map[string]string{"name": "fakephone", "vault_id": *vault})
		if err != nil {
			fail(err.Error())
		}
		if status != http.StatusOK {
			fail(fmt.Sprintf("配对失败：HTTP %d", status))
		}
		var body struct {
			DeviceToken string `json:"device_token"`
		}
		if err := json.Unmarshal(raw, &body); err != nil || body.DeviceToken == "" {
			fail("配对返回里没有 device_token")
		}
		fmt.Println(body.DeviceToken)
		return
	}

	if strings.TrimSpace(*token) == "" {
		fail("需要 -token（设备令牌）")
	}
	if !*deny && strings.TrimSpace(*secret) == "" && strings.TrimSpace(*ca) == "" {
		fail("需要 -secret（要回传的明文）或 -ca（签证书）")
	}

	pending, err := waitForPending(*broker, *token, *item, *wait)
	if err != nil {
		fail(err.Error())
	}
	fmt.Printf("看到待批准 %s（mode=%s，%s）\n", pending.Item, pending.Mode, pending.RequestID)

	payload := ""
	if !*deny {
		if strings.TrimSpace(pending.SealPublicKey) == "" {
			fail("这条请求没带 seal_public_key，spike 不支持服务端取值")
		}
		pub, err := boxpayload.ParsePublic(pending.SealPublicKey)
		if err != nil {
			fail("seal_public_key 解析失败")
		}
		var plaintext []byte
		if pending.Mode == "sign" {
			plaintext, err = signCert(*ca, pending)
			if err != nil {
				fail(err.Error())
			}
		} else {
			plaintext = []byte(*secret)
		}
		payload, err = boxpayload.Seal(pub, pending.RequestID, plaintext)
		if err != nil {
			fail("密封失败")
		}
	}

	decision := "approve"
	if *deny {
		decision = "deny"
	}
	body := map[string]string{"decision": decision}
	if payload != "" {
		body["payload"] = payload
	}
	status, err := post(*broker, "/v1/decision/"+pending.RequestID, *token, body)
	if err != nil {
		fail(err.Error())
	}
	fmt.Printf("已提交 %s，Broker 返回 %d\n", decision, status)
}

// signCert 用手机替身当 CA：与 App 的 SshCert.signUser 出同一种证书形状。
func signCert(caPath string, pending *pendingView) ([]byte, error) {
	material, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("CA 私钥读不了：%v", err)
	}
	ca, err := ssh.ParsePrivateKey(material)
	clearBytes(material)
	if err != nil {
		return nil, fmt.Errorf("CA 私钥不是 OpenSSH 格式：%v", err)
	}
	publicKey, _, _, rest, err := ssh.ParseAuthorizedKey([]byte(pending.PublicKey))
	if err != nil || len(strings.TrimSpace(string(rest))) != 0 {
		return nil, fmt.Errorf("请求里的 public_key 解析失败")
	}
	certTTL := pending.CertTTL
	if certTTL <= 0 {
		certTTL = 300
	}
	now := time.Now()
	certificate := &ssh.Certificate{
		Nonce:           make([]byte, 32),
		Key:             publicKey,
		CertType:        ssh.UserCert,
		KeyId:           "easy-unlocker/" + pending.RequestID,
		ValidPrincipals: []string{pending.SSHUser},
		// 与 App SshCert 同步：回拨 120s 吸收目标机时钟差（回拨不足 → sshd
		// 报 "Certificate invalid: not yet valid"）。
		ValidAfter: uint64(now.Add(-120 * time.Second).Unix()),
		ValidBefore:     uint64(now.Add(time.Duration(certTTL) * time.Second).Unix()),
		Permissions:     ssh.Permissions{Extensions: defaultExtensions()},
	}
	if _, err := rand.Read(certificate.Nonce); err != nil {
		return nil, fmt.Errorf("生成 nonce 失败")
	}
	if pending.SSHUser == "" {
		return nil, fmt.Errorf("请求没带 ssh_user")
	}
	if err := certificate.SignCert(rand.Reader, ca); err != nil {
		return nil, fmt.Errorf("签发失败：%v", err)
	}
	return ssh.MarshalAuthorizedKey(certificate), nil
}

// defaultExtensions 与 App 的 SshCert.DEFAULT_EXTENSIONS 同一组 permit-*——
// 空扩展段的证书在 sshd 那边拿不到 pty 等能力。
func defaultExtensions() map[string]string {
	out := make(map[string]string, 5)
	for _, name := range []string{
		"permit-X11-forwarding",
		"permit-agent-forwarding",
		"permit-port-forwarding",
		"permit-pty",
		"permit-user-rc",
	} {
		out[name] = ""
	}
	return out
}

func clearBytes(value []byte) {
	for i := range value {
		value[i] = 0
	}
}

func waitForPending(broker, token, item string, timeout time.Duration) (*pendingView, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		raw, status, err := get(broker, "/v1/device/pending", token)
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("取待批准失败：HTTP %d", status)
		}
		var views []pendingView
		if err := json.Unmarshal(raw, &views); err != nil {
			return nil, fmt.Errorf("待批准列表解析失败")
		}
		for i := range views {
			if views[i].State != "waiting" {
				continue
			}
			if item == "" || views[i].Item == item {
				return &views[i], nil
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return nil, fmt.Errorf("等了 %s 也没看到待批准", timeout)
}

func get(broker, path, token string) ([]byte, int, error) {
	request, err := http.NewRequest(http.MethodGet, strings.TrimRight(broker, "/")+path, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("请求构造失败")
	}
	request.Header.Set("Authorization", "Bearer "+token)
	return send(request)
}

func post(broker, path, token string, body any) (int, error) {
	_, status, err := postRaw(broker, path, token, body)
	return status, err
}

func postRaw(broker, path, token string, body any) ([]byte, int, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, 0, fmt.Errorf("请求编码失败")
	}
	request, err := http.NewRequest(http.MethodPost, strings.TrimRight(broker, "/")+path, bytes.NewReader(raw))
	if err != nil {
		return nil, 0, fmt.Errorf("请求构造失败")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	return send(request)
}

func send(request *http.Request) ([]byte, int, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, 0, fmt.Errorf("连接 Broker 失败：%v", err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	return raw, response.StatusCode, nil
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}

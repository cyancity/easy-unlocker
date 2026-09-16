package protocol

import (
	"errors"
	"fmt"
	"strings"
)

// maxCertTTLSeconds 是 sign 请求能要的最长证书有效期（24 小时）。请求想挂多久等
// 批准由 TTL 控制，证书有效多久由 CertTTL 控制，两者独立。
const maxCertTTLSeconds = 86400

const (
	ModeSign  = "sign"
	ModeWrite = "write"

	// Delivery 说明值怎么交到请求方。write 模式才有意义。
	//   DeliveryFile（默认，空串等价）：写到 target 指定的文件，内容会留在那台机器上。
	//   DeliveryEphemeral：只交给请求方进程（环境变量 / 匿名 fd），用完即焚，不落盘。
	DeliveryFile      = "file"
	DeliveryEphemeral = "ephemeral"

	StatusApproved = "approved"
	StatusDenied   = "denied"
	StatusExpired  = "expired"
	StatusFailed   = "failed"
)

// Request is the wire format for POST /v1/request. The public_key and ssh_user
// fields are optional M3 additions used only for SSH certificate requests;
// they never contain a private key.
type Request struct {
	Item      string `json:"item"`
	Mode      string `json:"mode"`
	Purpose   string `json:"purpose"`
	TTL       int    `json:"ttl"`
	Target    string `json:"target"`
	Requester string `json:"requester"`

	// Delivery 只描述放出方式，供手机展示与用户确认；Broker 不据此做任何加密动作。
	Delivery string `json:"delivery,omitempty"`

	PublicKey string `json:"public_key,omitempty"`
	SSHUser   string `json:"ssh_user,omitempty"`

	// CertTTL 是 sign 请求想要的证书有效期（秒）；0 = 由签发方用默认值。
	// 与 TTL（请求挂多久等批准）是独立的两个时间。
	CertTTL int `json:"cert_ttl,omitempty"`

	// SealPublicKey is the CLI's one-time X25519 public key (raw URL base64).
	// When set, the phone encrypts the secret to this key; Broker only forwards.
	SealPublicKey string `json:"seal_public_key,omitempty"`

	// RequestKey 是客户端生成的幂等键：长轮询被代理/断网掐断后，CLI 带同 key
	// 重发即可续等原请求，不会在手机上重复出现。空 = 老客户端，断线即取消。
	RequestKey string `json:"request_key,omitempty"`
}

func (r Request) Validate(maxTTL int) error {
	if strings.TrimSpace(r.Item) == "" {
		return errors.New("item is required")
	}
	if len(r.Item) > 256 {
		return errors.New("item is too long")
	}
	if r.Mode != ModeSign && r.Mode != ModeWrite {
		return fmt.Errorf("mode must be %q or %q", ModeSign, ModeWrite)
	}
	if strings.TrimSpace(r.Purpose) == "" {
		return errors.New("purpose is required")
	}
	if len(r.Purpose) > 4096 {
		return errors.New("purpose is too long")
	}
	if r.TTL < 1 {
		return errors.New("ttl must be positive")
	}
	if maxTTL > 0 && r.TTL > maxTTL {
		return fmt.Errorf("ttl must be at most %d seconds", maxTTL)
	}
	if len(r.Target) > 4096 {
		return errors.New("target is too long")
	}
	if r.Delivery != "" && r.Delivery != DeliveryFile && r.Delivery != DeliveryEphemeral {
		return fmt.Errorf("delivery must be %q or %q", DeliveryFile, DeliveryEphemeral)
	}
	if len(r.Requester) > 512 {
		return errors.New("requester is too long")
	}
	if len(r.PublicKey) > 8192 {
		return errors.New("public_key is too long")
	}
	if len(r.SSHUser) > 256 {
		return errors.New("ssh_user is too long")
	}
	if len(r.SealPublicKey) > 128 {
		return errors.New("seal_public_key is too long")
	}
	if len(r.RequestKey) > 128 {
		return errors.New("request_key is too long")
	}
	if r.CertTTL < 0 || r.CertTTL > maxCertTTLSeconds {
		return fmt.Errorf("cert_ttl must be at most %d seconds", maxCertTTLSeconds)
	}
	// 服务端没有 CA：sign 只能由手机签发并密封回来，少了这两样请求永远批不下来。
	if r.Mode == ModeSign {
		if strings.TrimSpace(r.PublicKey) == "" {
			return errors.New("public_key is required for sign")
		}
		if strings.TrimSpace(r.SealPublicKey) == "" {
			return errors.New("seal_public_key is required for sign")
		}
	}
	return nil
}

// Response is the wire format returned by the long-poll request. payload is
// always an application-encrypted envelope, never a plaintext credential.
type Response struct {
	Status    string `json:"status"`
	Mode      string `json:"mode,omitempty"`
	Payload   string `json:"payload,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	Message   string `json:"message,omitempty"`
}

type Decision struct {
	Decision string `json:"decision"`
	Sig      string `json:"sig,omitempty"`
	Payload  string `json:"payload,omitempty"`
}

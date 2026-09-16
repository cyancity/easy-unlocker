package boxpayload

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

const (
	envelopeVersion = "v2"
	publicKeySize   = 32
	privateKeySize  = 32
	nonceSize       = 12
)

// KeyPair is an X25519 key. The private key must be cleared after use.
type KeyPair struct {
	Private [privateKeySize]byte
	Public  [publicKeySize]byte
}

func Generate() (KeyPair, error) {
	var pair KeyPair
	if _, err := rand.Read(pair.Private[:]); err != nil {
		return KeyPair{}, errors.New("cannot create box key")
	}
	pub, err := curve25519.X25519(pair.Private[:], curve25519.Basepoint)
	if err != nil {
		pair.Clear()
		return KeyPair{}, errors.New("cannot derive box public key")
	}
	copy(pair.Public[:], pub)
	return pair, nil
}

func ParsePublic(encoded string) ([publicKeySize]byte, error) {
	var pub [publicKeySize]byte
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(raw) != publicKeySize {
		return pub, errors.New("invalid box public key")
	}
	copy(pub[:], raw)
	return pub, nil
}

func (k KeyPair) PublicBase64() string {
	return base64.RawURLEncoding.EncodeToString(k.Public[:])
}

func (k *KeyPair) Clear() {
	for i := range k.Private {
		k.Private[i] = 0
	}
	for i := range k.Public {
		k.Public[i] = 0
	}
}

// Seal encrypts plaintext to recipientPub. The envelope includes an ephemeral
// sender public key so only the matching private key can open it.
func Seal(recipientPub [publicKeySize]byte, requestID string, plaintext []byte) (string, error) {
	if strings.TrimSpace(requestID) == "" {
		return "", errors.New("missing payload binding")
	}
	sender, err := Generate()
	if err != nil {
		return "", err
	}
	defer sender.Clear()
	shared, err := curve25519.X25519(sender.Private[:], recipientPub[:])
	if err != nil {
		return "", errors.New("cannot complete box agreement")
	}
	key, err := deriveBoxKey(shared, requestID)
	clear(shared)
	if err != nil {
		return "", err
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", errors.New("cannot initialize box cipher")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", errors.New("cannot initialize box authenticator")
	}
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return "", errors.New("cannot create box nonce")
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, []byte(requestID))
	raw := make([]byte, 0, publicKeySize+nonceSize+len(ciphertext))
	raw = append(raw, sender.Public[:]...)
	raw = append(raw, nonce...)
	raw = append(raw, ciphertext...)
	return envelopeVersion + "." + base64.RawURLEncoding.EncodeToString(raw), nil
}

func Open(recipient KeyPair, requestID, envelope string) ([]byte, error) {
	if strings.TrimSpace(requestID) == "" {
		return nil, errors.New("missing payload binding")
	}
	version, encoded, ok := strings.Cut(envelope, ".")
	if !ok || version != envelopeVersion || encoded == "" {
		return nil, errors.New("invalid box envelope")
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(raw) < publicKeySize+nonceSize+16 {
		return nil, errors.New("invalid box encoding")
	}
	var senderPub [publicKeySize]byte
	copy(senderPub[:], raw[:publicKeySize])
	nonce := raw[publicKeySize : publicKeySize+nonceSize]
	ciphertext := raw[publicKeySize+nonceSize:]
	shared, err := curve25519.X25519(recipient.Private[:], senderPub[:])
	if err != nil {
		return nil, errors.New("cannot complete box agreement")
	}
	key, err := deriveBoxKey(shared, requestID)
	clear(shared)
	if err != nil {
		return nil, err
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.New("cannot initialize box cipher")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.New("cannot initialize box authenticator")
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, []byte(requestID))
	if err != nil {
		return nil, errors.New("box authentication failed")
	}
	return plaintext, nil
}

func deriveBoxKey(shared []byte, requestID string) ([]byte, error) {
	reader := hkdf.New(sha256.New, shared, []byte("easy-unlocker/v2/box/"), []byte(requestID))
	key := make([]byte, 32)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, errors.New("cannot derive box key")
	}
	return key, nil
}

func clear(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

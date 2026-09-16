package securepayload

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

const envelopeVersion = "v1"

// Seal protects a credential with a key derived from the pairing token and
// request ID. HTTPS should still be used in deployment; this envelope keeps a
// plaintext credential out of intermediary HTTP/proxy buffers as well.
func Seal(pairingToken, requestID string, plaintext []byte) (string, error) {
	if pairingToken == "" || requestID == "" {
		return "", errors.New("missing payload binding")
	}
	key := deriveKey(pairingToken, requestID)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", errors.New("cannot initialize payload cipher")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", errors.New("cannot initialize payload authenticator")
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", errors.New("cannot create payload nonce")
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, []byte(requestID))
	envelope := make([]byte, 0, len(nonce)+len(ciphertext))
	envelope = append(envelope, nonce...)
	envelope = append(envelope, ciphertext...)
	return envelopeVersion + "." + base64.RawURLEncoding.EncodeToString(envelope), nil
}

func Open(pairingToken, requestID, envelope string) ([]byte, error) {
	if pairingToken == "" || requestID == "" {
		return nil, errors.New("missing payload binding")
	}
	const separator = "."
	version, encoded, ok := splitEnvelope(envelope, separator)
	if !ok || version != envelopeVersion {
		return nil, errors.New("invalid payload envelope")
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("invalid payload encoding")
	}
	key := deriveKey(pairingToken, requestID)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.New("cannot initialize payload cipher")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.New("cannot initialize payload authenticator")
	}
	if len(raw) < gcm.NonceSize()+gcm.Overhead() {
		return nil, errors.New("invalid payload size")
	}
	nonce := raw[:gcm.NonceSize()]
	ciphertext := raw[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, []byte(requestID))
	if err != nil {
		return nil, errors.New("payload authentication failed")
	}
	return plaintext, nil
}

func deriveKey(pairingToken, requestID string) []byte {
	mac := hmac.New(sha256.New, []byte(pairingToken))
	mac.Write([]byte("easy-unlocker/v1/payload/"))
	mac.Write([]byte(requestID))
	return mac.Sum(nil)
}

func splitEnvelope(value, separator string) (string, string, bool) {
	for i := 0; i < len(value); i++ {
		if value[i:i+1] == separator {
			if i == 0 || i == len(value)-1 {
				return "", "", false
			}
			return value[:i], value[i+1:], true
		}
	}
	return "", "", false
}

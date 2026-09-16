package boxpayload

import (
	"bytes"
	"strings"
	"testing"
)

func TestSealOpenRoundTripAndBinding(t *testing.T) {
	recipient, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	defer recipient.Clear()
	plaintext := []byte("phone-vault-secret")
	envelope, err := Seal(recipient.Public, "req-1", plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(envelope, "v2.") {
		t.Fatalf("envelope=%q", envelope)
	}
	if strings.Contains(envelope, string(plaintext)) {
		t.Fatal("plaintext leaked into envelope")
	}
	opened, err := Open(recipient, "req-1", envelope)
	if err != nil || !bytes.Equal(opened, plaintext) {
		t.Fatalf("open: err=%v value=%q", err, opened)
	}
	other, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	defer other.Clear()
	if _, err := Open(other, "req-1", envelope); err == nil {
		t.Fatal("wrong recipient opened box")
	}
	if _, err := Open(recipient, "req-2", envelope); err == nil {
		t.Fatal("wrong request id opened box")
	}
}

func TestParsePublicRejectsShortKey(t *testing.T) {
	if _, err := ParsePublic("abc"); err == nil {
		t.Fatal("expected error")
	}
}

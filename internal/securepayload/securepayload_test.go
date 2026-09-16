package securepayload

import (
	"strings"
	"testing"
)

func TestSealOpenBindsTokenAndRequestID(t *testing.T) {
	plaintext := []byte("secret-value")
	envelope, err := Seal("pairing-token", "request-1", plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(envelope, string(plaintext)) {
		t.Fatal("plaintext appeared in envelope")
	}
	opened, err := Open("pairing-token", "request-1", envelope)
	if err != nil || string(opened) != string(plaintext) {
		t.Fatalf("open: err=%v length=%d", err, len(opened))
	}
	if _, err := Open("wrong-token", "request-1", envelope); err == nil {
		t.Fatal("wrong token opened payload")
	}
	if _, err := Open("pairing-token", "request-2", envelope); err == nil {
		t.Fatal("wrong request ID opened payload")
	}
	lastIndex := strings.IndexByte(envelope, '.') + 2
	last := envelope[lastIndex]
	if last == 'a' {
		last = 'b'
	} else {
		last = 'a'
	}
	tampered := envelope[:lastIndex] + string(last) + envelope[lastIndex+1:]
	if _, err := Open("pairing-token", "request-1", tampered); err == nil {
		t.Fatal("tampered payload opened")
	}
}

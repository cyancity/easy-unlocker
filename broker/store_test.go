package broker

import (
	"context"
	"testing"
)

func TestRBWStoreRejectsOptionLikeItem(t *testing.T) {
	_, err := (RBWStore{Binary: "/nonexistent/rbw"}).Get(context.Background(), "-f")
	if err == nil || err.Error() != "invalid item name" {
		t.Fatalf("err=%v, want \"invalid item name\"", err)
	}
}

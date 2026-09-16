package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cyancity/easy-unlocker/broker"
	"github.com/cyancity/easy-unlocker/internal/protocol"
)

type integrationNotifier struct {
	ch chan broker.Notification
}

func (n *integrationNotifier) Notify(_ context.Context, notification broker.Notification) error {
	n.ch <- notification
	return nil
}

type integrationAudit struct{}

func (integrationAudit) Record(broker.AuditEvent) error { return nil }

func TestClientWritePathEndToEndWithoutPlaintextResponse(t *testing.T) {
	const secret = "fixture-value-for-mvp-test"
	notifier := &integrationNotifier{ch: make(chan broker.Notification, 1)}
	server, err := broker.NewServer(broker.ServerConfig{
		PairingToken: "pairing-token",
		Store:        broker.NewMapStore(map[string][]byte{"OPENAI_API_KEY": []byte(secret + "\n")}),
		Notifier:     notifier,
		Audit:        integrationAudit{},
		MaxTTL:       time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	target := filepath.Join(t.TempDir(), ".env")
	request := protocol.Request{
		Item:      "OPENAI_API_KEY",
		Mode:      protocol.ModeWrite,
		Purpose:   "CLI 集成测试",
		TTL:       10,
		Target:    target,
		Requester: "agent@test",
	}
	resultCh := make(chan protocol.Response, 1)
	go func() {
		result, requestErr := (Client{BrokerURL: httpServer.URL, PairingToken: "pairing-token"}).Request(context.Background(), request)
		if requestErr != nil {
			resultCh <- protocol.Response{Status: "test-error", Message: requestErr.Error()}
			return
		}
		resultCh <- result
	}()
	notification := <-notifier.ch
	decisionBody, err := json.Marshal(protocol.Decision{Decision: "approve", Sig: notification.ApproveSig})
	if err != nil {
		t.Fatal(err)
	}
	decisionRequest, err := http.NewRequest(http.MethodPost, httpServer.URL+"/v1/decision/"+notification.RequestID, strings.NewReader(string(decisionBody)))
	if err != nil {
		t.Fatal(err)
	}
	decisionRequest.Header.Set("Content-Type", "application/json")
	decisionResponse, err := http.DefaultClient.Do(decisionRequest)
	if err != nil {
		t.Fatal(err)
	}
	decisionResponse.Body.Close()
	if decisionResponse.StatusCode != http.StatusOK {
		t.Fatalf("decision status=%d", decisionResponse.StatusCode)
	}
	result := <-resultCh
	if result.Status != protocol.StatusApproved || strings.Contains(result.Payload, secret) {
		t.Fatalf("unexpected approved response status=%q", result.Status)
	}
	value, err := OpenPayload(result, "pairing-token")
	if err != nil {
		t.Fatal(err)
	}
	value = NormalizeSecret(value)
	defer ClearBytes(value)
	if err := WriteSecret(target, value); err != nil {
		t.Fatal(err)
	}
	material, err := os.ReadFile(target)
	if err != nil || string(material) != secret {
		t.Fatalf("target contents mismatch: length=%d err=%v", len(material), err)
	}
	if mode := fileMode(t, target).Perm(); mode != 0o600 {
		t.Fatalf("target mode=%o", mode)
	}
	if hidden, _ := filepath.Glob(filepath.Join(filepath.Dir(target), ".easy-unlocker-write-*")); len(hidden) != 0 {
		t.Fatalf("temporary files remain: %v", hidden)
	}
}

func TestLoadSettingsRejectsWorldReadableConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("broker_url=http://127.0.0.1:8787\npairing_token=fixture-token\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSettings(path); err == nil {
		t.Fatal("accepted a non-private config file")
	}
}

func TestCheckBrokerURL(t *testing.T) {
	cases := []struct {
		raw     string
		wantErr bool
	}{
		{"https://arm.example", false},
		{"http://127.0.0.1:8787", false},
		{"http://localhost:8787", false},
		{"http://[::1]:8787", false},
		{"http://10.0.0.5:8787", true},
		{"ftp://x", true},
		{"", true},
	}
	for _, c := range cases {
		err := CheckBrokerURL(c.raw)
		if (err != nil) != c.wantErr {
			t.Fatalf("CheckBrokerURL(%q) err=%v, wantErr=%v", c.raw, err, c.wantErr)
		}
	}
	t.Setenv("EASY_UNLOCKER_ALLOW_HTTP", "1")
	if err := CheckBrokerURL("http://10.0.0.5:8787"); err != nil {
		t.Fatalf("allow-http override err=%v", err)
	}
	t.Setenv("EASY_UNLOCKER_ALLOW_HTTP", "")
	_, err := (Client{BrokerURL: "http://10.0.0.5:1", PairingToken: "t"}).Request(context.Background(), protocol.Request{
		Item: "x", Mode: protocol.ModeWrite, Purpose: "p", TTL: 1, Target: "/tmp/x",
	})
	want := CheckBrokerURL("http://10.0.0.5:1")
	if err == nil || want == nil || err.Error() != want.Error() {
		t.Fatalf("request err=%v, want %v", err, want)
	}
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode()
}

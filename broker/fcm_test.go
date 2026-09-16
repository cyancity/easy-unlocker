package broker

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFCMPayloadCarriesRequestAndGateway(t *testing.T) {
	raw, err := json.Marshal(fcmPayload("device-token", "req-123", "https://broker.example.com/"))
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Message struct {
			Token        string            `json:"token"`
			Notification map[string]string `json:"notification"`
			Data         map[string]string `json:"data"`
			Android      struct {
				Priority     string            `json:"priority"`
				Notification map[string]string `json:"notification"`
			} `json:"android"`
		} `json:"message"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Message.Token != "device-token" {
		t.Fatalf("token=%q", payload.Message.Token)
	}
	if payload.Message.Data["request_id"] != "req-123" {
		t.Fatalf("request_id=%q", payload.Message.Data["request_id"])
	}
	// 结尾斜杠要去掉，否则 App 拿它跟配对里的 URL 对不上
	if payload.Message.Data["gateway"] != "https://broker.example.com" {
		t.Fatalf("gateway=%q", payload.Message.Data["gateway"])
	}
	if payload.Message.Android.Notification["channel_id"] != "unlock_v2" {
		t.Fatalf("channel_id=%q", payload.Message.Android.Notification["channel_id"])
	}
	// 推送文案和载荷里都不能出现条目名 / 路径 / 用途
	body := string(raw)
	for _, leak := range []string{"OPENAI", "/tmp/", "修复"} {
		if strings.Contains(body, leak) {
			t.Fatalf("payload leaked %q: %s", leak, body)
		}
	}
}

func TestFCMPayloadOmitsEmptyGateway(t *testing.T) {
	raw, err := json.Marshal(fcmPayload("device-token", "req-123", ""))
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Message struct {
			Data map[string]string `json:"data"`
		} `json:"message"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload.Message.Data["gateway"]; ok {
		t.Fatal("gateway should be omitted when not configured")
	}
}

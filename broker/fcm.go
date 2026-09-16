package broker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const fcmScope = "https://www.googleapis.com/auth/firebase.messaging"

type FCMClient struct {
	project string
	// gateway 是自己的公网地址，会写进推送载荷：App 靠它知道这条请求来自哪台网关，
	// 点通知时能自动切过去。没有就留空（旧 App 不看这个字段）。
	gateway string
	http    *http.Client
}

func NewFCMClient(credentialsFile, gateway string) (*FCMClient, error) {
	path := strings.TrimSpace(credentialsFile)
	if path == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("cannot read FCM credentials")
	}
	var meta struct {
		ProjectID string `json:"project_id"`
	}
	if json.Unmarshal(raw, &meta) != nil || meta.ProjectID == "" {
		return nil, errors.New("FCM credentials missing project_id")
	}
	creds, err := google.CredentialsFromJSON(context.Background(), raw, fcmScope)
	if err != nil {
		return nil, errors.New("invalid FCM credentials")
	}
	return &FCMClient{
		project: meta.ProjectID,
		gateway: strings.TrimRight(strings.TrimSpace(gateway), "/"),
		http:    &http.Client{Timeout: 10 * time.Second, Transport: &oauth2.Transport{Source: creds.TokenSource}},
	}, nil
}

// fcmPayload 是纯函数，方便测：载荷里不写条目名、路径和用途，只有 request_id 与网关标识。
func fcmPayload(deviceToken, requestID, gateway string) map[string]any {
	data := map[string]string{"request_id": requestID}
	if strings.TrimSpace(gateway) != "" {
		data["gateway"] = strings.TrimRight(strings.TrimSpace(gateway), "/")
	}
	return map[string]any{
		"message": map[string]any{
			"token": deviceToken,
			"notification": map[string]string{
				"title": "easy-unlocker",
				"body":  "有一条待批准的请求",
			},
			"data": data,
			"android": map[string]any{
				"priority": "HIGH",
				"notification": map[string]string{
					"channel_id": "unlock_v2",
				},
			},
		},
	}
}

func (c *FCMClient) Send(ctx context.Context, deviceToken string, notification Notification) error {
	if c == nil || strings.TrimSpace(deviceToken) == "" {
		return errors.New("missing FCM device token")
	}
	body, err := json.Marshal(fcmPayload(deviceToken, notification.RequestID, c.gateway))
	if err != nil {
		return errors.New("cannot encode FCM payload")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://fcm.googleapis.com/v1/projects/"+c.project+"/messages:send", bytes.NewReader(body))
	if err != nil {
		return errors.New("cannot create FCM request")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.New("FCM send failed")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("FCM send returned an error")
	}
	return nil
}

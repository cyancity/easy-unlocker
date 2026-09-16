package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/cyancity/easy-unlocker/broker"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "broker:", err)
		os.Exit(1)
	}
}

// version 由 release.sh 用 -X main.version=<tag> 注入；源码构建是 "dev"。
var version = "dev"

func run() error {
	flags := flag.NewFlagSet("broker", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	showVersion := flags.Bool("version", false, "print build version and exit")
	listen := flags.String("listen", envOr("EASY_UNLOCKER_LISTEN", "127.0.0.1:8787"), "HTTP listen address")
	pairingToken := flags.String("pairing-token", "", "request pairing token")
	adminToken := flags.String("admin-token", "", "admin pending-list token")
	auditPath := flags.String("audit-log", envOr("EASY_UNLOCKER_AUDIT_LOG", "logs/broker.jsonl"), "audit JSONL path")
	storeName := flags.String("store", envOr("EASY_UNLOCKER_STORE", "rbw"), "secret store: rbw or mock")
	rbwBinary := flags.String("rbw", envOr("EASY_UNLOCKER_RBW", "rbw"), "rbw executable")
	mockItem := flags.String("mock-item", os.Getenv("EASY_UNLOCKER_MOCK_ITEM"), "development-only mock item")
	mockSecret := flags.String("mock-secret", "", "development-only mock value")
	maxTTL := flags.Duration("max-ttl", envDuration("EASY_UNLOCKER_MAX_TTL", time.Hour), "maximum request lifetime")
	tlsCert := flags.String("tls-cert", os.Getenv("EASY_UNLOCKER_TLS_CERT"), "TLS certificate path (optional)")
	tlsKey := flags.String("tls-key", os.Getenv("EASY_UNLOCKER_TLS_KEY"), "TLS private key path (optional)")
	deviceFile := flags.String("device-file", envOr("EASY_UNLOCKER_DEVICE_FILE", "data/devices.json"), "paired device token file")
	fcmCreds := flags.String("fcm-credentials", os.Getenv("EASY_UNLOCKER_FCM_CREDENTIALS"), "Firebase service account JSON")
	publicURL := flags.String("public-url", os.Getenv("EASY_UNLOCKER_PUBLIC_URL"), "本 Broker 的公网地址，写进推送载荷当网关标识（App 用它自动切换）")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	if *showVersion {
		fmt.Println(version)
		return nil
	}
	if *pairingToken == "" {
		*pairingToken = os.Getenv("EASY_UNLOCKER_PAIRING_TOKEN")
	}
	if *adminToken == "" {
		*adminToken = os.Getenv("EASY_UNLOCKER_ADMIN_TOKEN")
	}
	if *mockSecret == "" {
		*mockSecret = os.Getenv("EASY_UNLOCKER_MOCK_SECRET")
	}
	if *maxTTL < time.Second {
		return errors.New("max-ttl must be at least one second")
	}
	if (*tlsCert == "") != (*tlsKey == "") {
		return errors.New("--tls-cert and --tls-key must be provided together")
	}
	if strings.TrimSpace(*pairingToken) == "" {
		return errors.New("--pairing-token or EASY_UNLOCKER_PAIRING_TOKEN is required")
	}

	store, err := buildStore(*storeName, *rbwBinary, *mockItem, *mockSecret)
	if err != nil {
		return err
	}
	audit, err := broker.NewAuditLogger(*auditPath)
	if err != nil {
		return err
	}
	defer audit.Close()

	fcm, err := broker.NewFCMClient(*fcmCreds, *publicURL)
	if err != nil {
		return err
	}

	server, err := broker.NewServer(broker.ServerConfig{
		PairingToken: *pairingToken,
		AdminToken:   *adminToken,
		Store:        store,
		Audit:        audit,
		MaxTTL:       *maxTTL,
		DeviceFile:   *deviceFile,
		FCM:          fcm,
		Version:      version,
	})
	if err != nil {
		return err
	}

	httpServer := &http.Server{
		Addr:              *listen,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		// Long-poll /v1/request holds the connection until TTL. ReadTimeout/IdleTimeout
		// at 10s/2m cancel the client and drop the pending request.
		IdleTimeout:    70 * time.Minute,
		MaxHeaderBytes: 16 << 10,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() {
		var serveErr error
		if *tlsCert != "" {
			serveErr = httpServer.ListenAndServeTLS(*tlsCert, *tlsKey)
		} else {
			serveErr = httpServer.ListenAndServe()
		}
		if err := serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()
	select {
	case err := <-errCh:
		if err != nil {
			return err
		}
		return nil
	case <-ctx.Done():
		server.ClosePending()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	}
}

func buildStore(name, rbwBinary, mockItem, mockSecret string) (broker.SecretStore, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "rbw":
		return broker.RBWStore{Binary: rbwBinary}, nil
	case "mock":
		if mockItem == "" {
			return nil, errors.New("--mock-item or EASY_UNLOCKER_MOCK_ITEM is required for mock store")
		}
		return broker.NewMapStore(map[string][]byte{mockItem: []byte(mockSecret)}), nil
	default:
		return nil, errors.New("unsupported store; use rbw or mock")
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

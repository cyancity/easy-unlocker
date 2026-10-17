//go:build windows

package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cyancity/easy-unlocker/cli"
)

func TestWindowsCLIRoundTrip(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "agent workspace")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(directory, "easyGet.exe")
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}

	httpServer, notifications := listBroker(t)
	config := filepath.Join(directory, "config")
	if err := cli.WriteConfig(config, httpServer.URL, "pairing-token"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EASY_UNLOCKER_CONFIG", config)
	t.Setenv("EASY_UNLOCKER_BROKER_URL", "")
	t.Setenv("EASY_UNLOCKER_PAIRING_TOKEN", "")

	runCLI := func(args ...string) (string, error) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		output, err := exec.CommandContext(ctx, binary, args...).CombinedOutput()
		return string(output), err
	}
	if output, err := runCLI("ping", "--no-proxy"); err != nil {
		t.Fatalf("ping: %v\n%s", err, output)
	}

	const secret = "windows-fixture-value%&!\nsecond line"
	runApproved := func(args ...string) (string, error) {
		t.Helper()
		type result struct {
			output string
			err    error
		}
		done := make(chan result, 1)
		go func() {
			output, err := runCLI(args...)
			done <- result{output, err}
		}()
		select {
		case notification := <-notifications:
			view := pendingViewFor(t, httpServer.URL, notification.RequestID)
			approveWith(t, httpServer.URL, view.RequestID, notification.ApproveSig,
				sealAsPhone(t, view.SealPublicKey, view.RequestID, secret))
		case early := <-done:
			t.Fatalf("CLI exited before requesting approval: %v\n%s", early.err, early.output)
		case <-time.After(10 * time.Second):
			t.Fatal("CLI did not request approval")
		}
		got := <-done
		if strings.Contains(got.output, secret) {
			t.Fatal("CLI printed the secret")
		}
		return got.output, got.err
	}

	t.Run("write-and-replace", func(t *testing.T) {
		target := filepath.Join(directory, "secret file.env")
		for range 2 {
			output, err := runApproved("env", "fixture", "--write-to", target,
				"--for", "Windows native regression", "--ttl", "10", "--no-proxy")
			if err != nil {
				t.Fatalf("write-to: %v\n%s", err, output)
			}
			material, err := os.ReadFile(target)
			if err != nil || !bytes.Equal(material, []byte(secret)) {
				t.Fatalf("write-to did not preserve the approved bytes: %v", err)
			}
		}
	})

	t.Run("exec-and-cleanup", func(t *testing.T) {
		probe := filepath.Join(directory, "probe.ps1")
		marker := filepath.Join(directory, "carrier-path.txt")
		script := `$ErrorActionPreference = 'Stop'
if ($env:FIXTURE_SECRET -ne [IO.File]::ReadAllText($env:EASYGET_SECRET_FD)) { exit 51 }
if ($env:EASYGET_ITEM -ne 'fixture') { exit 52 }
[IO.File]::WriteAllText($env:PROBE_MARKER, $env:EASYGET_SECRET_FD)
exit 7
`
		if err := os.WriteFile(probe, []byte(script), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PROBE_MARKER", marker)
		command := `powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "` + probe + `"`
		output, err := runApproved("env", "fixture", "--exec", command, "--env-name", "FIXTURE_SECRET",
			"--for", "Windows native regression", "--ttl", "10", "--no-proxy")
		exit, ok := err.(*exec.ExitError)
		if !ok || exit.ExitCode() != 7 {
			t.Fatalf("exec exit = %v, want 7\n%s", err, output)
		}
		carrier, err := os.ReadFile(marker)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(string(carrier)); !os.IsNotExist(err) {
			t.Fatalf("secret carrier remains after CLI exit: %v", err)
		}
	})
}

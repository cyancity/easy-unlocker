//go:build windows

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Windows 侧只验证输入校验与 cmd /C 透传退出码；fd 语义差异见 exec_windows.go 注释。
func TestRunWithSecretWindowsValidation(t *testing.T) {
	if _, err := RunWithSecret("   ", []byte("value"), "item", ""); err == nil {
		t.Fatal("空命令应当报错")
	}
	if _, err := RunWithSecret("exit /B 0", []byte("value"), "item", "1BAD"); err == nil {
		t.Fatal("非法环境变量名应当报错")
	}
	if _, err := RunWithSecret("exit /B 0", []byte("value"), "item", "with-dash"); err == nil {
		t.Fatal("含短横线的环境变量名应当报错")
	}
}

func TestRunWithSecretWindowsUnicodeCommandAndCleanup(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "中文 with spaces")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMP", dir)
	t.Setenv("TEMP", dir)
	input := filepath.Join(dir, "输入.txt")
	output := filepath.Join(dir, "输出.txt")
	if err := os.WriteFile(input, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, err := RunWithSecret(fmt.Sprintf("copy /b %q %q >nul\r\nexit /b 7", input, output), []byte("fixture-secret"), "fixture", "")
	if err != nil || code != 7 {
		t.Fatalf("RunWithSecret exit=%d err=%v, want 7", code, err)
	}
	if got, err := os.ReadFile(output); err != nil || string(got) != "fixture" {
		t.Fatalf("Unicode command failed: output length=%d err=%v", len(got), err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("temporary command or secret carrier remains: entries=%d err=%v", len(entries), err)
	}
}

func TestRunWithSecretWindowsFileReadableByPowerShell(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMP", dir)
	t.Setenv("TEMP", dir)
	code, err := RunWithSecret(`powershell.exe -NoProfile -NonInteractive -Command "$ErrorActionPreference='Stop'; if ([IO.File]::ReadAllText($env:EASYGET_SECRET_FD) -ne 'fixture-secret') { exit 9 }; exit 7"`, []byte("fixture-secret"), "fixture", "")
	if err != nil || code != 7 {
		t.Fatalf("PowerShell could not read carrier: exit=%d err=%v, want 7", code, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary command or secret carrier remains: entries=%d err=%v", len(entries), err)
	}
}

func TestRunWithSecretWindowsProvidesEnvAndFile(t *testing.T) {
	out := filepath.Join(t.TempDir(), "probe.txt")
	t.Setenv("PROBE_OUT", out)
	code, err := RunWithSecret(
		`echo %EASYGET_SECRET%>"%PROBE_OUT%" & type "%EASYGET_SECRET_FD%">>"%PROBE_OUT%" & echo %EASYGET_ITEM%>>"%PROBE_OUT%"`,
		[]byte("s3cr3t"), "demo-item", "",
	)
	if err != nil {
		t.Fatalf("RunWithSecret: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read probe: %v", err)
	}
	content := string(got)
	for _, want := range []string{"s3cr3t", "demo-item"} {
		if !strings.Contains(content, want) {
			t.Fatalf("probe = %q，缺少 %q", content, want)
		}
	}
	if strings.Count(content, "s3cr3t") < 2 {
		t.Fatalf("probe = %q，env 与文件载体应各出现一次值", content)
	}
}

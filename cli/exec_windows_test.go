//go:build windows

package cli

import (
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

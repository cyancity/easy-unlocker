package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func countAnonFiles(t *testing.T) int {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(os.TempDir(), ".easyget-*"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	return len(matches)
}

// 子进程要同时能拿到：环境变量里的值、以及 /dev/fd/3 这个匿名 fd。
func TestRunWithSecretProvidesEnvAndAnonymousFd(t *testing.T) {
	out := filepath.Join(t.TempDir(), "probe.txt")
	t.Setenv("PROBE_OUT", out)
	before := countAnonFiles(t)

	code, err := RunWithSecret(
		`printf '%s' "$EASYGET_SECRET" > "$PROBE_OUT"; printf '|' >> "$PROBE_OUT"; cat /dev/fd/3 >> "$PROBE_OUT"; printf '|%s' "$EASYGET_ITEM" >> "$PROBE_OUT"`,
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
	if want := "s3cr3t|s3cr3t|demo-item"; string(got) != want {
		t.Fatalf("probe = %q, want %q", string(got), want)
	}
	if after := countAnonFiles(t); after != before {
		t.Fatalf("匿名载体残留：before=%d after=%d", before, after)
	}
}

func TestRunWithSecretHonoursEnvName(t *testing.T) {
	out := filepath.Join(t.TempDir(), "probe.txt")
	t.Setenv("PROBE_OUT", out)

	code, err := RunWithSecret(`printf '%s' "$MY_KEY" > "$PROBE_OUT"`, []byte("value"), "item", "MY_KEY")
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
	if string(got) != "value" {
		t.Fatalf("probe = %q, want %q", string(got), "value")
	}
}

func TestRunWithSecretForwardsExitCode(t *testing.T) {
	code, err := RunWithSecret("exit 7", []byte("value"), "item", "")
	if err != nil {
		t.Fatalf("RunWithSecret: %v", err)
	}
	if code != 7 {
		t.Fatalf("exit code = %d, want 7", code)
	}
}

func TestRunWithSecretRejectsBadInput(t *testing.T) {
	if _, err := RunWithSecret("   ", []byte("value"), "item", ""); err == nil {
		t.Fatal("空命令应当报错")
	}
	if _, err := RunWithSecret("true", []byte("value"), "item", "1BAD"); err == nil {
		t.Fatal("非法环境变量名应当报错")
	}
	if _, err := RunWithSecret("true", []byte("value"), "item", "with-dash"); err == nil {
		t.Fatal("含短横线的环境变量名应当报错")
	}
}

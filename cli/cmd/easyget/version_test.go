package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionDoesNotReadConfig(t *testing.T) {
	config := filepath.Join(t.TempDir(), "broken-config")
	if err := os.WriteFile(config, []byte("invalid config line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EASY_UNLOCKER_CONFIG", config)
	for _, arg := range []string{"version", "--version"} {
		t.Run(arg, func(t *testing.T) {
			printed := captureStdout(t)
			if err := run([]string{arg}); err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(printed()); got != version {
				t.Fatalf("version output = %q, want %q", got, version)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v2026.09.16", "v2026.09.16", 0},
		{"v2026.09.17", "v2026.09.16", 1},
		{"v2026.09.16", "v2026.09.17", -1},
		{"v2026.09.16-2", "v2026.09.16", 1},
		{"v2026.09.16-2", "v2026.09.16-3", -1},
		{"v2026.10.01", "v2026.09.30", 1},
		{"v2027.01.01", "v2026.12.31", 1},
		{"dev", "v2026.09.16", 0},
		{"v2026.09.16", "dev", 0},
		{"dev", "dev", 0},
		{"", "v2026.09.16", 0},
		{"v2026.9.6", "v2026.09.16", 0}, // 不补零的非法格式不可比
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

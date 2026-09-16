package main

import "testing"

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

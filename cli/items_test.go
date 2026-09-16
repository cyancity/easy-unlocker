package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseItemListAcceptsOnlyTheListShape(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		wantErr bool
	}{
		{"名单", `{"v":1,"items":[{"name":"openai","aliases":["OPENAI_API_KEY"]}]}`, false},
		{"库里没有条目也算答案", `{"v":1,"items":[]}`, false},
		{"没有 items 键", `{"v":1}`, true},
		{"版本不认识", `{"v":2,"items":[]}`, true},
		{"名字是空白", `{"v":1,"items":[{"name":"  "}]}`, true},
		// 老版本 App 会把保留名当普通请求渲染，用户手选一条后收到的就是一条真实凭据。
		{"一条真实凭据", `sk-live-0123456789`, true},
		{"被引号包起来的一条凭据", `"sk-live-0123456789"`, true},
		{"裸 JSON 数组", `[{"name":"openai"}]`, true},
		{"空载荷", ``, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			list, err := ParseItemList([]byte(tc.payload))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got list %+v", list)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if list.Version != itemListVersion {
				t.Fatalf("version=%d", list.Version)
			}
		})
	}
}

func TestSaveItemsRoundTripIsPrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "easy-unlocker")
	path := filepath.Join(dir, "items.json")
	saved := ItemList{
		FetchedAt: "2026-09-15T01:02:03Z",
		Items: []ItemEntry{
			{Name: "my-api-key", Aliases: []string{"OPENAI_API_KEY"}},
			{Name: "my-service-token"},
		},
	}
	if err := SaveItems(path, saved); err != nil {
		t.Fatal(err)
	}
	if mode := fileMode(t, path).Perm(); mode != 0o600 {
		t.Fatalf("cache mode=%o, want 0600", mode)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(dir, ".easy-unlocker-write-*")); len(leftovers) != 0 {
		t.Fatalf("temporary files remain: %v", leftovers)
	}

	loaded, err := LoadItems(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.FetchedAt != saved.FetchedAt {
		t.Fatalf("fetched_at=%q, want %q", loaded.FetchedAt, saved.FetchedAt)
	}
	if len(loaded.Items) != 2 || loaded.Items[0].Name != "my-api-key" || len(loaded.Items[0].Aliases) != 1 {
		t.Fatalf("round trip lost entries: %+v", loaded.Items)
	}
}

func TestLoadItemsReportsMissingAndCorruptCacheDifferently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "items.json")
	if _, err := LoadItems(path); !os.IsNotExist(err) {
		t.Fatalf("missing cache should surface os.ErrNotExist, got %v", err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadItems(path)
	if err == nil || os.IsNotExist(err) {
		t.Fatalf("corrupt cache should be its own error, got %v", err)
	}
}

func TestItemsCachePathSitsNextToTheConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "nested", "config")
	if got, want := ItemsCachePath(configPath), filepath.Join(dir, "nested", "items.json"); got != want {
		t.Fatalf("path=%q, want %q", got, want)
	}
	t.Setenv("EASY_UNLOCKER_CONFIG", configPath)
	if got, want := ItemsCachePath(""), filepath.Join(dir, "nested", "items.json"); got != want {
		t.Fatalf("env path=%q, want %q", got, want)
	}
}

// 缓存里永远只有名字和别名：别的东西一个字节都不该出现。
func TestSavedCacheCarriesNoValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "items.json")
	if err := SaveItems(path, ItemList{FetchedAt: "2026-09-15T01:02:03Z", Items: []ItemEntry{{Name: "openai"}}}); err != nil {
		t.Fatal(err)
	}
	material, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(material, &raw); err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	for key := range raw {
		keys[key] = true
	}
	for _, allowed := range []string{"v", "fetched_at", "items"} {
		if !keys[allowed] {
			t.Fatalf("cache is missing %q: %s", allowed, material)
		}
	}
	if len(keys) != 3 {
		t.Fatalf("cache has unexpected keys %v", keys)
	}
}

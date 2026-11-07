package main

import "testing"

// 放哪一栏的优先级：前端选的那栏 → 请求方指定的栏 → 旧规矩（有密码放密码、没密码放备注）。
//
// 最要紧的是「请求方点名了栏、那一栏却是空的」这种情况：不能悄悄换成另一栏，
// 否则会把另一个值当成请求方要的东西交出去。桌面端原来就是这么错的——
// 请求里没有栏这个概念，条目形状没加载出来时一律放密码，值在备注里的条目直接失败。
func TestReleaseField(t *testing.T) {
	both := &vaultItem{Name: "my-token", Secret: "short-password-value", Note: "long-note-value-in-note-field"}
	onlyNote := &vaultItem{Name: "n", Note: "note-only"}
	onlySecret := &vaultItem{Name: "s", Secret: "secret-only"}
	empty := &vaultItem{Name: "e"}

	cases := []struct {
		name      string
		chosen    string
		requested string
		item      *vaultItem
		want      string
	}{
		{"前端选了备注就用备注", "note", "", both, "note"},
		{"前端选了密码就用密码", "secret", "", both, "secret"},
		{"前端没选、请求方点名备注", "", "note", both, "note"},
		{"前端没选、请求方点名密码", "", "password", both, "secret"},
		{"协议里的 password 翻成前端的 secret", "", "password", onlySecret, "secret"},
		{"都没给、有密码就放密码（旧规矩）", "", "", both, "secret"},
		{"都没给、只有备注就放备注（旧规矩）", "", "", onlyNote, "note"},
		{"都没给、两栏都空就放密码（旧规矩）", "", "", empty, "secret"},
		{"前端选的优先于请求方", "secret", "note", both, "secret"},
		{"前端给了不认识的值就当没给", "bogus", "note", both, "note"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := releaseField(tc.chosen, tc.requested, tc.item); got != tc.want {
				t.Fatalf("releaseField(%q, %q) = %q, want %q", tc.chosen, tc.requested, got, tc.want)
			}
		})
	}
}

// 请求方点名了备注、备注却是空的：releaseField 仍然选备注，
// 由 Approve 报错放行——不能改成放密码。
func TestReleaseFieldKeepsRequestedEmptyColumn(t *testing.T) {
	item := &vaultItem{Name: "my-token", Secret: "short-password-value", Note: ""}
	if got := releaseField("", "note", item); got != "note" {
		t.Fatalf("releaseField = %q, want note（指定栏是空的也不能换栏）", got)
	}
}

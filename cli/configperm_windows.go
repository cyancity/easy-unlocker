//go:build windows

package cli

import (
	"os"
	"os/exec"
	"os/user"
)

// checkConfigPrivate Windows 版：Go 在 Windows 上对普通文件一律报 0666/0444，
// unix 位检查没有意义，真正的隔离由 NTFS ACL 承担（%USERPROFILE% 下本就按用户隔离）。
func checkConfigPrivate(info os.FileInfo) error {
	return nil
}

// hardenPrivateFile 尽力把文件 ACL 收紧到仅当前用户：关继承、只授当前 SID 完全控制。
// grantee 必须用 SID（*S-1-5-…）而不是用户名——本机用户的名字解析不可靠。
// 失败不阻断：配置写在用户目录，默认 ACL 已按用户隔离。
func hardenPrivateFile(path string) {
	current, err := user.Current()
	if err != nil || current.Uid == "" {
		return
	}
	_ = exec.Command("icacls", path, "/inheritance:r", "/grant:r", "*"+current.Uid+":(F)").Run()
}

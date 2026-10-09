//go:build !windows

package cli

import (
	"errors"
	"os"
)

// checkConfigPrivate 拒绝 group/other 可读的配置文件。
func checkConfigPrivate(info os.FileInfo) error {
	if info.Mode().Perm()&0o077 != 0 {
		return errors.New("config file is not private")
	}
	return nil
}

// hardenPrivateFile 写私有文件后兜底收紧权限（0600）。
func hardenPrivateFile(path string) {
	_ = os.Chmod(path, 0o600)
}

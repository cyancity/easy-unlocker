//go:build !windows

package cli

import (
	"errors"
	"os"
)

// checkConfigPrivate 拒绝 group/other 可读的配置文件。
func checkConfigPrivate(_ string, info os.FileInfo) error {
	if info.Mode().Perm()&0o077 != 0 {
		return errors.New("config file is not private")
	}
	return nil
}

func hardenPrivateFile(path string) error {
	return os.Chmod(path, 0o600)
}

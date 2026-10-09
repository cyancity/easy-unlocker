package cli

import "regexp"

const (
	// 子进程里承载密钥的环境变量默认名；用 --env-name 可覆盖。
	defaultSecretEnvName = "EASYGET_SECRET"
	itemEnvName          = "EASYGET_ITEM"
	fdEnvName            = "EASYGET_SECRET_FD"
)

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

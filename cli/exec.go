package cli

import (
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

const (
	// 子进程里承载密钥的环境变量默认名；用 --env-name 可覆盖。
	defaultSecretEnvName = "EASYGET_SECRET"
	// 匿名 fd 在子进程里的路径。cmd.ExtraFiles 从 3 开始编号。
	anonymousFDPath = "/dev/fd/3"
	itemEnvName     = "EASYGET_ITEM"
	fdEnvName       = "EASYGET_SECRET_FD"
)

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// RunWithSecret 把 secret 只交给子进程，不落盘，并返回子进程的退出码。
//
// 子进程拿到两种取用形态（同一个 --exec 里都给，调用方按需取）：
//
//	$EASYGET_SECRET（可用 envName 改名）  值的本体，给从环境变量读密钥的程序
//	$EASYGET_SECRET_FD                    /dev/fd/3，匿名 fd 的路径，给必须吃"文件路径"的工具
//
// 匿名 fd 的做法：写临时文件 → 立刻 os.Remove 抹掉名字 → 经 cmd.ExtraFiles 传给子进程。
// 文件系统里搜不到它，子进程退出后即不可访问。
//
// 诚实边界：macOS 没有 memfd_create，unlink 之后虽然不可寻址，但物理磁盘可能留有已释放
// 块；这里不声称"绝对不过磁盘"。另外值进了环境变量，同用户的其他进程可经 /proc/<pid>/environ
// 读到，适合本机单人使用的场景。
func RunWithSecret(command string, secret []byte, item string, envName string) (int, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return 0, errors.New("--exec 的命令不能为空")
	}
	if envName == "" {
		envName = defaultSecretEnvName
	}
	if !envNamePattern.MatchString(envName) {
		return 0, errors.New("--env-name 只能由字母、数字、下划线组成，且不能以数字开头")
	}
	file, err := anonymousSecretFile(secret)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	cmd := exec.Command("sh", "-c", command)
	cmd.Env = append(os.Environ(),
		envName+"="+string(secret),
		itemEnvName+"="+item,
		fdEnvName+"="+anonymousFDPath,
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.ExtraFiles = []*os.File{file}

	err = cmd.Run()
	if err == nil {
		return 0, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), nil
	}
	return 0, errors.New("无法执行 --exec 的命令")
}

// anonymousSecretFile 把值写成"没有名字"的临时文件：写完立刻 unlink，只留 fd。
func anonymousSecretFile(secret []byte) (*os.File, error) {
	file, err := os.CreateTemp(os.TempDir(), ".easyget-*")
	if err != nil {
		return nil, errors.New("无法创建临时载体")
	}
	name := file.Name()
	discard := func() {
		_ = file.Close()
		_ = os.Remove(name)
	}
	if err := file.Chmod(0o600); err != nil {
		discard()
		return nil, errors.New("无法保护临时载体")
	}
	if _, err := file.Write(secret); err != nil {
		discard()
		return nil, errors.New("无法写入临时载体")
	}
	if err := file.Sync(); err != nil {
		discard()
		return nil, errors.New("无法持久化临时载体")
	}
	if err := os.Remove(name); err != nil {
		discard()
		return nil, errors.New("无法抹掉临时载体的名字")
	}
	if _, err := file.Seek(0, 0); err != nil {
		_ = file.Close()
		return nil, errors.New("无法复位临时载体")
	}
	return file, nil
}

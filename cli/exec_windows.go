//go:build windows

package cli

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

// RunWithSecret Windows 版：没有 sh、没有 /dev/fd、cmd.ExtraFiles 也不支持。
// $EASYGET_SECRET_FD 给一个真实临时文件路径（%TEMP% 本用户私有，用完即删）。
//
// 命令不直接走 cmd /C：<command>：cmd 对 /C 后的整串做引号剥离，
// 只要命令里带引号就会解析错乱。改写进临时 .cmd 批处理文件再执行，
// 内容逐字节保留，语义与 cmd /C 一致（含 &、>> 等）。
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
	file, err := namedSecretFile(secret)
	if err != nil {
		return 0, err
	}
	defer os.Remove(file.Name())
	defer file.Close()

	script, err := commandScript(command)
	if err != nil {
		return 0, err
	}
	defer os.Remove(script)

	cmd := exec.Command("cmd", "/C", script)
	cmd.Env = append(os.Environ(),
		envName+"="+string(secret),
		itemEnvName+"="+item,
		fdEnvName+"="+file.Name(),
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

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

// commandScript 把用户命令写进临时 .cmd 文件，返回路径。
// 只装命令文本，不含密钥——密钥经环境变量与 fd 文件交付。
func commandScript(command string) (string, error) {
	f, err := os.CreateTemp("", "easyget-*.cmd")
	if err != nil {
		return "", errors.New("无法创建临时命令脚本")
	}
	name := f.Name()
	if _, err := f.WriteString("@echo off\r\n" + command + "\r\n"); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return "", errors.New("无法写入临时命令脚本")
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return "", errors.New("无法写入临时命令脚本")
	}
	return name, nil
}

// namedSecretFile 写一个有名字的临时文件；Windows 下 fd 传不了，只能传路径。
func namedSecretFile(secret []byte) (*os.File, error) {
	file, err := os.CreateTemp(os.TempDir(), ".easyget-*")
	if err != nil {
		return nil, errors.New("无法创建临时载体")
	}
	name := file.Name()
	discard := func() {
		_ = file.Close()
		_ = os.Remove(name)
	}
	hardenPrivateFile(name)
	if _, err := file.Write(secret); err != nil {
		discard()
		return nil, errors.New("无法写入临时载体")
	}
	if err := file.Sync(); err != nil {
		discard()
		return nil, errors.New("无法持久化临时载体")
	}
	if _, err := file.Seek(0, 0); err != nil {
		discard()
		return nil, errors.New("无法复位临时载体")
	}
	return file, nil
}

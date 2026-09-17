package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/cyancity/easy-unlocker/cli"
	"github.com/cyancity/easy-unlocker/internal/boxpayload"
	"github.com/cyancity/easy-unlocker/internal/protocol"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		var exit exitError
		if errors.As(err, &exit) {
			os.Exit(exit.code)
		}
		fmt.Fprintln(os.Stderr, "easyGet:", err)
		os.Exit(1)
	}
}

// exitError 只用来把 --exec 子进程的退出码原样透出去。
type exitError struct{ code int }

func (e exitError) Error() string { return fmt.Sprintf("--exec 的子进程退出码 %d", e.code) }

func run(args []string) error {
	if len(args) == 0 {
		return usageError()
	}
	if isHelpArg(args[0]) {
		printHelp()
		return nil
	}
	// 子命令位置的 `-h`：`easyGet env -h` 里 "-h" 本来会被当成条目名。
	if len(args) > 1 && isHelpArg(args[1]) &&
		(args[0] == "env" || args[0] == "ssh" || args[0] == "ping" || args[0] == "list") {
		printHelp()
		return nil
	}
	configPath := configArg(args)
	settings, err := cli.LoadSettings(configPath)
	if err != nil {
		return err
	}
	if args[0] == "version" {
		fmt.Fprintln(os.Stdout, version)
		return nil
	}
	if args[0] == "ping" {
		return runPing(settings, args[1:])
	}
	if args[0] == "env" {
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			return errors.New("用法：easyGet env <item> --write-to <path> --for \"...\"（落盘）或 easyGet env <item> --exec \"<命令>\" --for \"...\"（不落盘）")
		}
		err := runEnv(settings, args[1], args[2:], "easyGet env")
		if err == nil {
			maybeNotifyUpdate(settings, configPath)
		}
		return err
	}
	if args[0] == "ssh" {
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			return errors.New("用法：easyGet ssh <name> --for \"...\"")
		}
		err := runSSH(settings, args[1], args[2:])
		if err == nil {
			maybeNotifyUpdate(settings, configPath)
		}
		return err
	}
	if args[0] == "list" {
		err := runList(settings, configPath, args[1:])
		if err == nil {
			maybeNotifyUpdate(settings, configPath)
		}
		return err
	}
	if args[0] == "pair" {
		return runPair(settings, configPath, args[1:])
	}
	return runGeneric(settings, args[0], args[1:])
}

func runPing(settings cli.Settings, args []string) error {
	flags := newCommonFlags("easyGet ping", settings)
	if err := flags.Parse(args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := (cli.Client{BrokerURL: flags.broker, PairingToken: flags.token, NoProxy: flags.noProxy}).Ping(ctx); err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, "正常：Broker 可达")
	// ping 是诊断命令，顺带直报版本对比（不走一天一次的限频）。
	client := cli.Client{BrokerURL: flags.broker, PairingToken: flags.token, NoProxy: flags.noProxy}
	latest, verr := client.BrokerVersion(ctx)
	switch {
	case verr == nil && isReleaseVersion(version) && isReleaseVersion(latest):
		if compareVersions(latest, version) > 0 {
			fmt.Fprintf(os.Stdout, "有新版本 %s（当前 %s）。升级：curl -L -o /tmp/easyGet \"https://github.com/cyancity/easy-unlocker/releases/latest/download/easyGet-$(uname -s | tr 'A-Z' 'a-z')-$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')\" && install -m 755 /tmp/easyGet ~/.local/bin/easyGet\n", latest, version)
		} else {
			fmt.Fprintln(os.Stdout, "已是最新版本："+version)
		}
	case verr == nil && latest == "":
		// 老 Broker 没有 /v1/version——跳过，不算异常。
	}
	return nil
}

// listPurpose 是刷新名单时写在批准页上的说明。老版本 App 不认识保留名，会把这句话
// 原样显示在一个普通批准页上，所以「不放出任何值」必须写进去。
const listPurpose = "列出本机可用的条目名（系统请求，不放出任何值）"

// runList 打本机缓存的条目名名单；只有 --refresh 才向手机要一份新的。
//
// 缓存只是告示牌，不是账本：手机端随时可能加过、删过、改过名，这份名单不参与匹配。
func runList(settings cli.Settings, configPath string, args []string) error {
	flags := newCommonFlags("easyGet list", settings)
	refresh := flags.Bool("refresh", false, "ask the phone for a fresh list (costs one approval)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	flags.applyTo(&settings)
	cachePath := cli.ItemsCachePath(configPath)
	if *refresh {
		return runListRefresh(settings, cachePath, flags)
	}
	list, err := cli.LoadItems(cachePath)
	if errors.Is(err, os.ErrNotExist) {
		return errors.New("这台机器还没有条目名名单。直接问用户条目名，或跑一次 easyGet list --refresh（手机会要一次指纹）")
	}
	if err != nil {
		return err
	}
	printItems(list)
	warnListAge(list)
	return nil
}

// runListRefresh 用保留条目名走一次普通请求，把手机封回来的名单写进缓存。
func runListRefresh(settings cli.Settings, cachePath string, flags *commonFlags) error {
	purpose := flags.purpose
	if strings.TrimSpace(purpose) == "" {
		purpose = listPurpose
	}
	request := protocol.Request{
		Item:      cli.ReservedListItem,
		Mode:      protocol.ModeWrite,
		Purpose:   purpose,
		TTL:       flags.ttl,
		Requester: flags.requester,
		Delivery:  protocol.DeliveryEphemeral,
	}
	payload, err := release(settings, request)
	if err != nil {
		return err
	}
	defer cli.ClearBytes(payload)
	list, err := cli.ParseItemList(payload)
	if err != nil {
		// 老版本 App 会把 #items 当普通请求渲染：用户手选一条，这里收到的就是一条真实
		// 凭据。所以解析失败时绝不打印、绝不落盘，只报错。
		return fmt.Errorf("手机回的载荷不像是条目名名单（%v）：已丢弃，没有落盘也没有打印。多半是 App 版本过旧，升级后重试", err)
	}
	list.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	if err := cli.SaveItems(cachePath, list); err != nil {
		return err
	}
	printItems(list)
	return nil
}

// printItems 把名单打到 stdout：一行一条，别名跟在括号里。stdout 只放名字，方便直接读；
// 时间与陈旧提示一律走 stderr。
func printItems(list cli.ItemList) {
	for _, entry := range list.Items {
		line := entry.Name
		if len(entry.Aliases) > 0 {
			line += " (" + strings.Join(entry.Aliases, ", ") + ")"
		}
		fmt.Fprintln(os.Stdout, line)
	}
}

// warnListAge 提醒这份名单有多旧，以及该拿它怎么办。
func warnListAge(list cli.ItemList) {
	when := "时间未知"
	if stamp, err := time.Parse(time.RFC3339, list.FetchedAt); err == nil {
		when = fmt.Sprintf("%s（%s前）", stamp.Local().Format("2006-01-02 15:04"), listAge(time.Since(stamp)))
	}
	fmt.Fprintf(os.Stderr, "这份名单取自 %s，共 %d 条。\n", when, len(list.Items))
	fmt.Fprintln(os.Stderr, "它是上一次的快照：手机端之后可能加过、删过、改过名。名字不在里面不等于条目不存在——用户给的名字直接请求就行；要一份新的用 easyGet list --refresh（一次指纹）。")
}

func listAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "不到 1 分钟"
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 小时", int(d.Hours()))
	default:
		return fmt.Sprintf("%d 天", int(d.Hours()/24))
	}
}

// runEnv 取一条凭据，两种放出方式二选一：
//
//	--write-to <path>  落盘到文件（内容会留在这台机器上）
//	--exec "<命令>"    值只交给子进程（环境变量 + 匿名 fd），用完即焚，不落盘
//
// 两种方式互斥；delivery 字段只用于让手机把后果显示给用户确认。
func runEnv(settings cli.Settings, item string, args []string, name string) error {
	flags := newCommonFlags(name, settings)
	writeTo := flagsSetString(flags, "write-to", "", "write the value to this file (stays on disk)")
	execCmd := flagsSetString(flags, "exec", "", "run this command with the value; nothing is written to disk")
	envName := flagsSetString(flags, "env-name", "", "env var carrying the value for --exec (default EASYGET_SECRET)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	flags.applyTo(&settings)
	if flags.purpose == "" {
		return errors.New("--for 是必需的")
	}
	target, delivery, err := resolveReleaseTarget(*writeTo, *execCmd)
	if err != nil {
		return err
	}

	request := protocol.Request{
		Item:      item,
		Mode:      protocol.ModeWrite,
		Purpose:   flags.purpose,
		TTL:       flags.ttl,
		Target:    target,
		Requester: flags.requester,
		Delivery:  delivery,
	}
	value, err := release(settings, request)
	if err != nil {
		return err
	}
	defer cli.ClearBytes(value)

	if delivery == protocol.DeliveryEphemeral {
		code, err := cli.RunWithSecret(*execCmd, value, item, *envName)
		if err != nil {
			return err
		}
		if code != 0 {
			return exitError{code: code}
		}
		return nil
	}
	if err := cli.WriteSecret(target, value); err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, "已完成")
	return nil
}

// resolveReleaseTarget 把 --write-to / --exec 解析成（落盘路径, delivery）。
// 两者互斥，且必须给一个：落盘会留在目标机器上，--exec 只交给子进程。
func resolveReleaseTarget(writeTo, execCommand string) (string, string, error) {
	toFile := strings.TrimSpace(writeTo) != ""
	toExec := strings.TrimSpace(execCommand) != ""
	switch {
	case toFile && toExec:
		return "", "", errors.New("--write-to 与 --exec 互斥：要么落盘到文件，要么只交给子进程")
	case !toFile && !toExec:
		return "", "", errors.New("必须给一个：--write-to <path>（落盘）或 --exec \"<命令>\"（不落盘）")
	case toExec:
		return "", protocol.DeliveryEphemeral, nil
	default:
		return writeTo, protocol.DeliveryFile, nil
	}
}

func runGeneric(settings cli.Settings, item string, args []string) error {
	if !hasFlag(args, "write-to") && !hasFlag(args, "exec") {
		return runSSH(settings, item, args)
	}
	return runEnv(settings, item, args, "easyGet")
}

// release 走完「请求 → 手机批准 → 解开信封」，把明文交给调用方；写不写盘由调用方决定。
func release(settings cli.Settings, request protocol.Request) ([]byte, error) {
	boxKey, err := boxpayload.Generate()
	if err != nil {
		return nil, errors.New("无法创建本次传输密钥")
	}
	defer boxKey.Clear()
	request.SealPublicKey = boxKey.PublicBase64()
	ctx, cancel := cli.RequestContext(request.TTL)
	defer cancel()
	client := cli.Client{BrokerURL: settings.BrokerURL, PairingToken: settings.PairingToken, NoProxy: settings.NoProxy}
	response, err := client.Request(ctx, request)
	if err != nil {
		return nil, err
	}
	switch response.Status {
	case protocol.StatusApproved:
		value, err := cli.OpenBoxPayload(response, boxKey)
		if err != nil {
			return nil, err
		}
		return cli.NormalizeSecret(value), nil
	case protocol.StatusDenied:
		fmt.Fprintln(os.Stdout, "已拒绝：手机端拒绝了本次请求")
		return nil, errors.New("request denied")
	case protocol.StatusExpired:
		fmt.Fprintln(os.Stdout, "已过期：批准超时")
		return nil, errors.New("request expired")
	default:
		return nil, errors.New("凭据发放失败")
	}
}

func runSSH(settings cli.Settings, item string, args []string) error {
	flags := newCommonFlags("easyGet ssh", settings)
	identityPath := flagsSetString(flags, "identity", "", "existing SSH private key to reuse")
	identityTo := flagsSetString(flags, "identity-to", "", "where to write the ephemeral identity key")
	certTo := flagsSetString(flags, "cert-to", "", "where to write the SSH certificate")
	sshUser := flagsSetString(flags, "ssh-user", defaultSSHUser(), "SSH certificate principal (default: local username)")
	certTTL := flags.Int("cert-ttl", 300, "seconds the issued SSH certificate stays valid (max 86400)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.purpose == "" {
		return errors.New("--for 是必需的")
	}
	if strings.TrimSpace(*sshUser) == "" {
		return errors.New("--ssh-user 不能为空")
	}
	if *certTTL < 1 {
		return errors.New("--cert-ttl 必须为正数")
	}
	if *certTo == "" {
		*certTo = defaultCertPath(item)
	}
	identity, err := cli.PrepareSSHIdentity(*identityPath, *identityTo, *certTo)
	if err != nil {
		return err
	}
	defer identity.Clear()
	boxKey, err := boxpayload.Generate()
	if err != nil {
		return errors.New("无法创建本次传输密钥")
	}
	defer boxKey.Clear()
	request := protocol.Request{
		Item:          item,
		Mode:          protocol.ModeSign,
		Purpose:       flags.purpose,
		TTL:           flags.ttl,
		Requester:     flags.requester,
		PublicKey:     identity.PublicKeyLine(),
		SSHUser:       *sshUser,
		CertTTL:       *certTTL,
		SealPublicKey: boxKey.PublicBase64(),
	}
	ctx, cancel := cli.RequestContext(request.TTL)
	defer cancel()
	response, err := (cli.Client{BrokerURL: flags.broker, PairingToken: flags.token, NoProxy: flags.noProxy}).Request(ctx, request)
	if err != nil {
		return err
	}
	switch response.Status {
	case protocol.StatusApproved:
		cert, err := cli.OpenBoxPayload(response, boxKey)
		if err != nil {
			return err
		}
		defer cli.ClearBytes(cert)
		cert = cli.NormalizeSecret(cert)
		if err := cli.VerifyCertificate(cert, identity, *certTTL, *sshUser); err != nil {
			return err
		}
		if err := cli.WritePublicFile(*certTo, append(cert, '\n')); err != nil {
			return err
		}
		if err := identity.SaveGenerated(); err != nil {
			return err
		}
		if len(identity.GeneratedPEM) > 0 {
			fmt.Fprintf(os.Stdout, "已完成：短时 SSH 证书已写入 %s，临时私钥已写入 %s\n", *certTo, identity.IdentityPath)
		} else {
			fmt.Fprintf(os.Stdout, "已完成：短时 SSH 证书已写入 %s\n", *certTo)
		}
		return nil
	case protocol.StatusDenied:
		fmt.Fprintln(os.Stdout, "已拒绝：手机端拒绝了本次请求")
		return errors.New("request denied")
	case protocol.StatusExpired:
		fmt.Fprintln(os.Stdout, "已过期：批准超时")
		return errors.New("request expired")
	default:
		return errors.New("凭据发放失败")
	}
}

type commonFlags struct {
	*flag.FlagSet
	purpose   string
	ttl       int
	requester string
	broker    string
	token     string
	noProxy   bool
}

// 把解析出来的连接参数写回 settings：runWrite/runGeneric 经由 settings 传参，
// 不写回的话 --broker/--token 会被静默忽略（runSSH/runPing 直接用 flags，没这个问题）。
func (f *commonFlags) applyTo(settings *cli.Settings) {
	settings.BrokerURL = f.broker
	settings.PairingToken = f.token
	settings.Requester = f.requester
	settings.NoProxy = f.noProxy
}

func newCommonFlags(name string, settings cli.Settings) *commonFlags {
	flags := &commonFlags{FlagSet: flag.NewFlagSet(name, flag.ContinueOnError)}
	flags.SetOutput(os.Stderr)
	flags.StringVar(&flags.purpose, "for", "", "why this credential is needed (shown on the phone)")
	flags.IntVar(&flags.ttl, "ttl", 300, "seconds the request stays approvable")
	flags.StringVar(&flags.requester, "requester", settings.Requester, "who is asking (default user@host)")
	flags.StringVar(&flags.broker, "broker", settings.BrokerURL, "broker base URL")
	flags.StringVar(&flags.token, "token", "", "pairing token")
	flags.BoolVar(&flags.noProxy, "no-proxy", false, "connect to the broker directly, ignoring http_proxy/https_proxy")
	flags.String("config", "", "config file path (already read at startup)")
	if flags.token == "" {
		flags.token = settings.PairingToken
	}
	return flags
}

func flagsSetString(flags *commonFlags, name, value, usage string) *string {
	return flags.String(name, value, usage)
}

func configArg(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "--config" && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(args[i], "--config=") {
			return strings.TrimPrefix(args[i], "--config=")
		}
	}
	return ""
}

func hasFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == "--"+name || strings.HasPrefix(arg, "--"+name+"=") {
			return true
		}
	}
	return false
}

// defaultSSHUser 给证书 principal 的默认值：本机用户名。SSH 登录用的哪个账号，
// 证书就该签给谁，所以默认取本机用户名最贴合实际用法。
func defaultSSHUser() string {
	if value := os.Getenv("USER"); value != "" {
		return value
	}
	if current, err := user.Current(); err == nil && current.Username != "" {
		return current.Username
	}
	return "unlocker"
}

func defaultCertPath(item string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "easy-unlocker-cert.pub"
	}
	return filepath.Join(home, ".ssh", "easy-unlocker-"+safeFilename(item)+"-cert.pub")
}

func safeFilename(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			builder.WriteRune(r)
		} else {
			builder.WriteByte('-')
		}
	}
	if builder.Len() == 0 {
		return "credential"
	}
	return builder.String()
}

// runPair 用手机 App 生成的一次性配对码，把这台机器换到一张设备令牌并写进配置。
// 配对之后这台机器就是一个独立的请求方：不再依赖全局 pairing token。
func runPair(settings cli.Settings, configPath string, args []string) error {
	flags := newCommonFlags("easyGet pair", settings)
	code := flagsSetString(flags, "code", "", "手机 App 生成的一次性配对码")
	name := flagsSetString(flags, "name", "", "这台机器的名字（默认 user@host）")
	if err := flags.Parse(args); err != nil {
		return err
	}
	flags.applyTo(&settings)
	if strings.TrimSpace(*code) == "" {
		return errors.New("需要 --code（手机 App → 设置 → 设备 → 添加设备，10 分钟内有效）")
	}
	// 手机设备表里显示的就是这个名字；不给就落到 Broker 的 "device" 兜底，等于没名字。
	if strings.TrimSpace(*name) == "" {
		*name = settings.Requester
	}
	token, deviceName, expiresAt, err := cli.PairClaim(settings.BrokerURL, *code, *name)
	if err != nil {
		return err
	}
	path := cli.DefaultConfigPath(configPath)
	if err := cli.WriteConfig(path, settings.BrokerURL, token); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "已配对：设备名 %s（手机「设备」页显示这个名字），令牌有效期至 %s\n配置写入 %s\n试一下：easyGet ping\n", deviceName, expiresAt, path)
	return nil
}

func usageError() error {
	return errors.New("missing arguments: run `easyGet --help`")
}

func isHelpArg(arg string) bool {
	return arg == "-h" || arg == "--help" || arg == "help"
}

// printHelp 是给 agent 和终端用户看的英文说明；CLI 的运行时状态消息仍是中文。
func printHelp() {
	fmt.Fprint(os.Stdout, `easyGet — fetch a credential from your phone, without reading it into the conversation.

HOW IT WORKS
  1. You ask for one item by name.
  2. Your phone shows who / what / why / how long, behind a fingerprint.
  3. On approval, the value is sealed to a one-time public key made by this
     process. The broker only forwards ciphertext. Nothing is printed to stdout.

USAGE
  easyGet env <item> --write-to <path> --for "<reason>"     release to a FILE
  easyGet env <item> --exec "<command>" --for "<reason>"    release to a PROCESS
  easyGet pair --code <code> --broker <url>                 pair this machine with a
                                                            one-time code from the app
  easyGet ssh <ca-item> --for "<reason>"                    short-lived SSH certificate
  easyGet list [--refresh]                                  item names cached on this machine
                                                            (--refresh asks the phone: one approval)
  easyGet ping                                              check broker reachability
                                                            (also reports a newer release)
  easyGet version                                           print this binary's version
  easyGet --help                                            this text

ITEM NAMES
  Usually you already know the name because the user told you. When you do not,
  "easyGet list" prints the names this machine learned last time: no approval, no
  network, one item per line, aliases in parentheses.

  The cache is a SIGNPOST, NOT A LEDGER. The phone's vault is the only authority and
  may have gained, lost or renamed items since. So a name MISSING from the list does
  NOT mean the item does not exist — if the user names it, ask for it anyway, do not
  answer "there is no such item".

  For a fresh list run "easyGet list --refresh". That is a real request: it wakes the
  phone and costs the user one approval, so use it sparingly, not before every fetch.

  "#items" is reserved for this handshake and cannot be used as an item name.

RELEASE MODES (choose exactly one)
  --write-to <path>   Writes the value to <path> (mode 0600) and prints "已完成".
                      The file STAYS on this machine. Parent directory must exist.
                      Use it only when something really needs a file on disk.

  --exec "<command>"  Never writes a file. Runs <command> with the value exposed as
                      environment variables and an anonymous file descriptor. The
                      child's exit code becomes easyGet's exit code.
                      PREFER THIS for API keys and private keys.

ENVIRONMENT GIVEN TO --exec
  EASYGET_SECRET       the value itself (rename with --env-name MY_VAR)
  EASYGET_ITEM         the item name you asked for
  EASYGET_SECRET_FD    /dev/fd/3 — an anonymous fd holding the same bytes, for tools
                       that insist on a file path (oci key_file, ssh -i, ...).
                       The backing file is unlinked immediately, so there is no path
                       to find on disk; the fd disappears when the child exits.

FLAGS
  --for "<reason>"     required; shown on the phone so the user knows why
  --ttl <seconds>      how long the request stays approvable (default 300, max 3600)
  --write-to <path>    release to this file
  --exec "<command>"   release to this command instead of a file
  --env-name <NAME>    env var name for the value in --exec (default EASYGET_SECRET)
  --requester <label>  who is asking (default user@host)
  --broker <url>       override the configured broker (https; plain http only for
                       127.0.0.1/localhost)
  --config <path>      override the config file location

EXAMPLES
  # API key for a one-off script: nothing left on disk
  easyGet env MY_API_KEY --exec 'python run.py' --for 'score the batch'

  # same, under the name the program expects
  easyGet env openai --env-name OPENAI_API_KEY --exec 'node job.js' --for 'nightly job'

  # a PEM that a tool needs as a path
  easyGet env oci-api-key --exec 'sh -c "oci --config-file $OCI_CONFIG compute instance list"' \
    --for 'list instances'

  # genuinely need a file (e.g. an env file another process reads)
  easyGet env my-service-token --write-to ~/.config/myapp/.env \
    --for 'service token for local dev'

PAIRING
  New machines no longer need the global pairing token copied over.
  1. Phone app: Settings -> Devices -> Add device. It shows a one-time code
     valid for 10 minutes.
  2. Run:  easyGet pair --broker <url> --code <code>   ...and this machine gets its
     own device token (valid 180 days, renewable from the app) written to the
     config file.
  Devices can be listed and revoked from the phone app at any time. Deleting the
  config file logs this machine out.

EXIT CODES
  0     approved and released (with --exec: the child exited 0)
  N     with --exec: the child's exit code
  1     anything else — denied, expired, broker unreachable, missing item,
        or "easyGet list" with nothing cached yet

NEVER
  Do not print, log, echo or cat the released value, and do not paste it into a
  chat. Read the exit status instead. Success prints only "已完成".

SSH CERTIFICATES
  easyGet ssh <ca-item> --for "<reason>"

  <ca-item> names the vault item holding your SSH CA private key (an OpenSSH
  ed25519 private key, usually in the item's note field). The phone signs a
  short-lived user certificate for an ephemeral identity — the CA key itself is
  never released.

  On approval two files land in ~/.ssh:
    easy-unlocker-<item>            ephemeral private key (mode 0600)
    easy-unlocker-<item>-cert.pub   the certificate (public)
  Then: ssh -i ~/.ssh/easy-unlocker-<item> <ssh-user>@<host>

  The target host needs your CA in TrustedUserCAKeys. The certificate is only
  checked at login: sessions already running keep working after it expires.

  --ssh-user <name>   principal to sign (default: local username)
  --cert-ttl <sec>    certificate lifetime (default 300, max 86400); this is the
                      login window — existing sessions are NOT cut when it ends
  --identity <path>   reuse an existing private key instead of generating one
  --cert-to <path>    where to write the certificate
  --identity-to <p>   where to write the generated identity
`)
}

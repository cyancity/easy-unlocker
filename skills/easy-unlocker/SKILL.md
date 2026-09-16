---
name: easy-unlocker
description: Request a credential (API key, private key, token) via the easyGet CLI — the user's phone approves with biometrics and the value goes only to a child process or a local file, never into the conversation. Use when you need an API key / OPENAI_API_KEY / credential / private key, hit a 401, see easyGet env / easyGet --exec, or the user says not to paste secrets into the chat.
user-invocable: false
---

# easy-unlocker (easyGet)

When the agent needs a credential, call `easyGet`. The phone approves. Plaintext never enters the conversation, logs, or memory.

**`easyGet --help` is the authoritative command reference.** This skill only covers how to choose and what not to do. Install and pairing: [`docs/AGENTS-SETUP.md`](https://github.com/cyancity/easy-unlocker/blob/main/docs/AGENTS-SETUP.md).

## Quick start

```bash
# preferred: nothing touches disk — the value goes into the child process only
easyGet env OPENAI_API_KEY --exec 'python run.py' --for 'batch scoring job'

# only when a file must stay on this machine
easyGet env my-service-token --write-to /tmp/app.env --for 'service token for local dev'
```

`--write-to` and `--exec` are mutually exclusive — exactly one is required. On success `easyGet` prints only `已完成` (with `--exec` you see the child's own output).

If `easyGet` is not on PATH, install the prebuilt binary:

```bash
os=$(uname -s | tr 'A-Z' 'a-z'); arch=$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')
curl -L -o /tmp/easyGet "https://github.com/cyancity/easy-unlocker/releases/latest/download/easyGet-${os}-${arch}"
install -m 755 /tmp/easyGet ~/.local/bin/easyGet && easyGet --help
```

## Choosing the release mode

| Situation | Use |
|---|---|
| Program reads the secret from env | `--exec`; rename with `--env-name NAME` |
| Program insists on a file path (oci `key_file`, `ssh -i`) | `--exec`, use `$EASYGET_SECRET_FD` inside the command |
| A file must stay on disk (e.g. `.env` another process reads) | `--write-to`, and accept that it persists |
| Private keys (PEM, key text) | prefer `--exec`; `--write-to` only if the user accepts disk |

`--exec` exposes to the child:

| Variable | Content |
|---|---|
| `EASYGET_SECRET` | the value itself (`--env-name` renames it) |
| `EASYGET_SECRET_FD` | `/dev/fd/3` — anonymous fd holding the same bytes; unlinked immediately, gone when the child exits |
| `EASYGET_ITEM` | the item name you asked for |

`--exec` propagates the child's exit code.

The phone's approval screen states the consequence up front ("will write to `<path>`" vs "never touches disk") — the user sees it before approving.

## Workflow

1. This is the only way to fetch a credential: no password-manager CLI, no asking the user to paste, no plaintext in chat.
2. `command -v easyGet`; missing, or `easyGet ping` fails → stop and read `docs/AGENTS-SETUP.md` (build, write `~/.config/easy-unlocker/config` mode 0600). Never print the pairing token into the conversation.
3. Pick a release mode (table above) and run it. Longer waits: `--ttl` up to 3600.
4. Wait for the phone: biometric → if the name doesn't match, the user picks the item by hand → approve.
5. Denied / expired / failed = non-zero exit. Do not proceed as if you hold the credential. Public-network long polls may be cut around 2 minutes (`Broker 请求失败`) — a known limitation, not a bug.

Don't know the item name? `easyGet list` prints the names cached on this machine — free, offline. The cache is a signpost, not a ledger: a missing name does NOT mean the item doesn't exist — if the user names it, request it anyway. `easyGet list --refresh` pulls a fresh list and costs the user one phone approval; don't refresh before every fetch.

## Degradation (easyGet unusable)

`command -v easyGet` fails, `easyGet ping` fails, or the request is denied / times out:

- Do NOT fall back to a password-manager CLI, and don't retry in a loop.
- Tell the user which item, why, and the target path — let the USER handle it on their own terminal.
- Never accept, echo, or store plaintext the user pastes into the chat.

## Anti-patterns

WRONG: `easyGet env KEY --write-to /tmp/x.env` then `cat`/`echo` it to inspect the value.
RIGHT: let the child command read the file, or use `--exec` so no file exists; success = exit code + `已完成`.

WRONG: `--write-to` for a private key (e.g. `~/.oci/oci_api_key.pem`).
RIGHT: `--exec 'sh -c "…uses $EASYGET_SECRET_FD…"'` — the key never hits the filesystem.

WRONG: after an easyGet failure, asking the user to paste the key or switching to another secrets tool.
RIGHT: use the degradation path above.

## Checklist

- [ ] No pairing token / recovery code / plaintext secret in chat or logs
- [ ] `--for` present, and exactly one of `--write-to` / `--exec`
- [ ] `--exec` preferred for private keys

## See also

- `easyGet --help` — authoritative reference (modes, env vars, flags, examples, exit codes)
- [`docs/AGENTS-SETUP.md`](https://github.com/cyancity/easy-unlocker/blob/main/docs/AGENTS-SETUP.md) — install, config, phone app
- [`README.md`](https://github.com/cyancity/easy-unlocker) — what the product is

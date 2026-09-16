<p align="center">
  <img src="assets/logo/easy-unlocker-mark.svg" alt="easy-unlocker" width="140">
</p>

<h1 align="center">easy-unlocker</h1>

<p align="center">
  <b>Your phone is the approval plane for agents.</b><br>
  When an agent needs a secret, it never reads it into the conversation.<br>
  One fingerprint, and the value arrives sealed to that one process.
</p>

<p align="center">
  <b>English</b> · <a href="README.md"><b>中文</b></a>
</p>

<p align="center">
  <a href="https://github.com/cyancity/easy-unlocker/releases/latest"><img src="https://img.shields.io/github/v/release/cyancity/easy-unlocker" alt="release"></a>
  <img src="https://img.shields.io/badge/license-Apache--2.0%20%2F%20AGPL--3.0-blue" alt="license">
  <img src="https://img.shields.io/badge/platform-Android%20·%20iOS%20·%20CLI-lightgrey" alt="platforms">
</p>

<p align="center">
  <a href="https://easy-unlocker.pages.dev/"><b>Site</b></a> ·
  <a href="#install-and-use">Install &amp; use</a> ·
  <a href="docs/AGENTS-SETUP.md">Agent runbook</a> ·
  <a href="docs/PROTOCOL.md">Protocol</a> ·
  <a href="SECURITY.md">Security model</a>
</p>

---

## Why

Every agent eventually reaches for a secret: an API key, an SSH private key, a token you'd rather not hand over.

The fast move is pasting it into the chat. Fast, and now the plaintext lives in the model context, the transcript, and whatever logs sit downstream. The careful move is opening a password manager, copying, pasting, then double-checking the field. Nine times out of ten the chat wins, because lazy is real.

easy-unlocker makes the safe path lazier than the paste:

```bash
easyGet env OPENAI_API_KEY --exec 'python run.py' --for "calling the openai api"
# → phone pops: who (user@laptop) · what (OPENAI_API_KEY) · why
# → fingerprint → the value lands in the python process env, never on disk
```

Each `easyGet` run mints a one-time keypair. The public key rides the request to your phone, the phone seals the value with it, and only the private key held by that one command can open it. **The relay in between sees ciphertext and nothing else.**

```
┌────────┐  ① request (+one-time pubkey) ┌────────┐  ② push/poll   ┌────────┐
│  CLI   │ ────────────────────────────▶ │ Broker │ ─────────────▶ │ Phone  │
│ easyGet│ ◀────── ④ sealed value ────── │ relays │ ◀── ③ approve+seal │ bio │
└────────┘                                └────────┘                 └────────┘
   only ①'s private key opens ④            ciphertext only           the only
                                                                         plaintext exit
```

> Not a password manager, not a Bitwarden replacement, no browser autofill. It only holds the few items you're willing to hand to an agent.

## Install and use

Three pieces:

| Component | What it does |
|---|---|
| Phone app (`android/` `ios/`) | Stores items, approves with fingerprint or password, signs SSH login certs on-device |
| `easyGet` CLI (`cli/`) | The entry point agents call: sends requests, receives sealed values, writes files or injects into processes |
| Broker (`broker/` `worker/`) | Pairing, per-vault routing, ciphertext relay. You run one yourself: a Cloudflare Worker by default (one `wrangler deploy`), or the Go binary on a VPS |

**1. Install easyGet** (prebuilt for macOS / Linux / Windows):

```bash
os=$(uname -s | tr 'A-Z' 'a-z'); arch=$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')
curl -L -o /tmp/easyGet "https://github.com/cyancity/easy-unlocker/releases/latest/download/easyGet-${os}-${arch}"
install -m 755 /tmp/easyGet ~/.local/bin/easyGet
```

On Windows, grab `easyGet-windows-amd64.exe` from [Releases](https://github.com/cyancity/easy-unlocker/releases/latest).

**2. Install the app**: on Android, install `easy-unlocker-debug.apk` from Releases. On iOS, the source is in `ios/`; open it in Xcode and install with your own signing. Then create a vault (the recovery code lands in a file on the device) and point Settings at a broker.

**3. Pair**: in the app, Settings → Devices → generate a pairing code (8 characters, valid 10 minutes), then on the computer:

```bash
easyGet pair --broker https://broker.example.com --code <8-char code>
easyGet ping   # reachable, and it tells you if a newer release exists
```

> **Tip**: if pushes don't reach your Android device, check notification permission, battery policy (set to "unrestricted"), and auto-start — many Android ROMs restrict background work by default.

**Using it** is three kinds of request:

```bash
# most common: the value goes into a child process env, nothing touches disk
easyGet env OPENAI_API_KEY --exec 'python run.py' --for "calling the openai api"

# want a file? say so (mode 0600)
easyGet env OCI_KEY --write-to ~/.oci/key.pem --for "oci cli reads key_file"

# don't know the item names? check the list first (free, doesn't buzz the phone)
easyGet list

# SSH login: the phone signs a cert valid for minutes, the CA key never leaves it
easyGet ssh my-ssh-ca --for "log into the server for diagnostics"
```

> **You're an agent / coding assistant?** Install, pairing, how to pick between the request modes, and what not to do: it's all in [docs/AGENTS-SETUP.md](docs/AGENTS-SETUP.md). Follow it, and never ask a human to paste you a secret.

## Features and status

### Working today

- Phone approvals: every request shows who asked, what for, and where the value lands. Approve with fingerprint or password, or deny.
- Two ways to hand over: write to a 0600 file, or inject into a child process env with nothing left on disk.
- Item name list: `easyGet list` lets the agent check names before asking, instead of guessing and buzzing your phone.
- Short-lived SSH certs: the phone signs a minutes-valid login certificate with a CA key stored in the vault. The private key never leaves the phone.
- Self-serve pairing: one-time 10-minute codes replace manual token copying. Paired devices can be listed, revoked, renewed, renamed.
- Vaults stay isolated: requests only reach the devices bound to yours, and a new phone kicks the old one offline.
- Multiple brokers: pair with several gateways, switch anytime; tapping a notification jumps to the one that sent it.
- Bitwarden backup import, encrypted backup export/import (device migration), recovery code, on-device approval history.
- Push: FCM on Android, reaches the lock screen; the app polls while it's open.
- `easyGet` checks versions itself and nudges you when a newer release exists.

### Where each side stands

| Side | State |
|---|---|
| Android | Feature complete, in daily use on real devices |
| iOS | Roughly at parity with Android and tested on a real phone; missing .eu1 import and APNs push |
| easyGet | Released for macOS, Linux, Windows |
| Broker | Two equivalent implementations (Go and Cloudflare Worker), both live |
| Landing page | Lives in its own repository (`site/` was split out) |

### Next

- iOS push: APNs needs a paid developer account; until then the iOS app polls while open
- Field-level release: hand over only the fields the agent asked for from a longer note
- CLI write-back: let agents deposit newly minted credentials into the vault
- Quotas and invites for sharing one broker across users; further out, hosted subscriptions

## Trust model in one line

> **You don't have to trust the broker.** It sees metadata (item names, purpose, device names); values and CA keys travel sealed end-to-end and it can't open them. Full threat model in [SECURITY.md](SECURITY.md); field-level protocol spec in [docs/PROTOCOL.md](docs/PROTOCOL.md).

## Docs

| Doc | Contents |
|---|---|
| [docs/AGENTS-SETUP.md](docs/AGENTS-SETUP.md) | Install/build/release runbook for coding assistants |
| [docs/PROTOCOL.md](docs/PROTOCOL.md) | HTTP protocol + sealed payload format + tenant model |
| [SECURITY.md](SECURITY.md) | Vulnerability reporting + threat model |
| [docs/PARITY.md](docs/PARITY.md) | Android/iOS/Broker/CLI feature tree |
| [docs/DEPLOY.md](docs/DEPLOY.md) | Self-host the broker |

## License

| Directory | License |
|---|---|
| `cli/` `android/` `ios/` `internal/` `docs/` etc. | [Apache-2.0](LICENSE) |
| `broker/` `worker/` | [AGPL-3.0](broker/LICENSE): self-host freely; offer it as a hosted service and your changes must be open-sourced |

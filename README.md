# trans

[![Go](https://img.shields.io/badge/go-1.26.6-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Write prompts in the language you think in and get them translated to English for your coding agent. A native Windows translation panel that opens over your terminal window.

## Why this exists

You think faster in your own language. But an agent answers in the language it was asked in, so your prompts end up in the replies, the comments, the commits and the docs. Translate the prompt and the drift has no source.

Side effect: your sentence and its English sit side by side, prompt after prompt. It rubs off.

## Features

- **Native Windows panel** — opens over your terminal window, no extra software needed
- **Multiple translation services** — DeepL, Google, MyMemory, any OpenAI-compatible API
- **No key required** — free services work without any API key
- **Live translation** — see the English as you write
- **Vim bindings** — modal editing for the draft box
- **Draft persistence** — your unfinished prompt is kept between sessions
- **Code protection** — backticked spans and fenced blocks are not translated
- **Sent-prompt history** — `ctrl+g` offers the prompts you have already sent

## Quick start

```bash
# Build for Windows
make windows

# Or build for your platform
go build -o bin/trans-window.exe ./cmd/trans-window
```

### Without an API key

The panel works as a prompt box even without translation. With no key configured, you write in your language and the draft is handed to the agent as-is.

### With translation

Create a config file:

```bash
# Windows
%APPDATA%\trans\.env

# Or set environment variables
TRANS_PROVIDER=gtranslate
```

For DeepL (free tier available):

```
TRANS_API_KEY=your-deepl-key
```

For any OpenAI-compatible API:

```
TRANS_PROVIDER=openai
TRANS_API_KEY=sk-...
TRANS_ENDPOINT=https://api.deepseek.com/v1
TRANS_MODEL=deepseek-chat
```

## Usage

```bash
# Open the panel over the window in front
trans-window open

# Open in review mode (types without sending)
trans-window open --review

# Open over a specific window
trans-window open --target 0x1a2b3c

# List available windows
trans-window list-windows

# Translate text directly (no panel)
trans-window translate "Hallo Welt"
```

### Windows daemon

`trans-windowd` waits for a hotkey and opens the panel:

```bash
# Start the daemon (default hotkey: ctrl+alt+t)
trans-windowd

# Use a custom hotkey
TRANS_HOTKEY=ctrl+shift+t trans-windowd
```

## Key bindings

| Key | Action |
| --- | --- |
| `alt+enter` | Translate and send (`ctrl+d` also works) |
| `enter` | New line |
| `ctrl+r` | Switch between sending and typing |
| `ctrl+l` | Toggle live translation |
| `ctrl+t` | Translate now / retry after error |
| `tab` | Read the full translation / back to draft |
| `ctrl+g` | Show the prompts already sent |
| `ctrl+u` | Discard draft |
| `esc` | Close (from normal mode if vim is on) |
| `ctrl+c` | Close always |

## Settings

Every setting can be a line in the `.env` file or an environment variable.

| Setting | Default | Meaning |
| --- | --- | --- |
| `TRANS_API_KEY` | none | Credentials for the translation service |
| `TRANS_PROVIDER` | auto | Which service: `deepl`, `google`, `gtranslate`, `openai`, `mymemory`, `cmd`, `dry-run`, `off` |
| `TRANS_MODEL` | service default | Model name for OpenAI-compatible APIs |
| `TRANS_PASTE_KEYS` | auto | Paste chord for the panel (`ctrl+v`, `ctrl+shift+v`) |
| `TRANS_COMMAND` | none | Local translation command |
| `TRANS_LANGUAGE` | `EN-US` | Target language |
| `TRANS_ENDPOINT` | service default | Override service endpoint |
| `TRANS_SUBMIT` | `1` | `0` types without sending |
| `TRANS_VIM` | `0` | `1` enables vim bindings |
| `TRANS_LIVE` | `1` | `0` translates only on send |
| `TRANS_CONFIRM` | `0` | `1` shows English before sending |
| `TRANS_KEEP_DRAFT` | `1` | `0` starts with empty box |
| `TRANS_MAX_DRAFT` | `2000` | Characters before warning |
| `TRANS_PULSE` | `1` | `0` stops the live circle animation |
| `TRANS_LOGO` | `1` | `0` hides the draft box signature |
| `TRANS_HOTKEY` | `ctrl+alt+t` | Hotkey for the daemon |
| `TRANS_HISTORY` | `1` | `0` keeps no record of sent prompts |
| `TRANS_HISTORY_LIMIT` | `500` | Prompts kept before the oldest are dropped |

### Services that need no key

```
TRANS_PROVIDER=gtranslate
```

- **`gtranslate`** — Google's public translate endpoint. Undocumented and can be rate limited.
- **`mymemory`** — Fallback with a smaller daily allowance.

## Local translation

Point the plugin at a program instead of a service:

```bash
TRANS_COMMAND=/path/to/translateLocally -m de-en-base
```

The draft is written to stdin, the translation read from stdout. No key, no network.

See [docs/local-translation.md](docs/local-translation.md) for details.

## Sent-prompt history

Every prompt that reached the agent is written down — what you wrote, what was
sent, which window it went into — and `ctrl+g` opens the record, newest first.
`enter` puts the selected prompt back into the box exactly as it was written,
`delete` drops one, `esc` closes. The record holds `TRANS_HISTORY_LIMIT`
prompts (500 by default), drops the oldest beyond that, and stays on your
machine in the state directory, written as privately as the drafts.

See [docs/history.md](docs/history.md) for the details.

## What leaves your machine

The draft goes to the translation service. Code in backticks and fenced blocks is held back and never sent. The API key goes to the translation service only.

A local translator (`TRANS_COMMAND`) keeps everything on your machine.

## Development

```bash
make qa       # formatting, linting, race tests, vulnerability scan
make build    # build for Windows
make windows  # cross-compile for Windows
```

See [docs/architecture.md](docs/architecture.md) for how the pieces fit together.

## Credits

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss).

Reference: [wazum/herdr-polyglot](https://github.com/wazum/herdr-polyglot)

## License

[MIT](LICENSE)

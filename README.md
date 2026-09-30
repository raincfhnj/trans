# trans

[![Go](https://img.shields.io/badge/go-1.26.6-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Write prompts in the language you think in and get them translated to English for your coding agent. A native Windows translation panel that opens over your terminal window.

## Why this exists

You think faster in your own language. But an agent answers in the language it was asked in, so your prompts end up in the replies, the comments, the commits and the docs. Translate the prompt and the drift has no source.

Side effect: your sentence and its English sit side by side, prompt after prompt. It rubs off.

## Features

- **Native Windows panel** — opens over your terminal window, no extra software needed
- **Selection translation** — select text in the terminal, press a chord, read the translation beside it
- **Settings window** — service, key, panel behaviour and chords, edited in a window summoned the same way
- **Multiple translation services** — DeepL, Google, MyMemory, any OpenAI-compatible API
- **No key required** — free services work without any API key
- **Live translation** — see the English as you write
- **Vim bindings** — modal editing for the draft box
- **Draft persistence** — your unfinished prompt is kept between sessions
- **Code protection** — backticked spans and fenced blocks are not translated
- **Translation memory** — a sentence paid for once is not paid for again, not even after a restart
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

# Translate what is selected in the window in front
trans-window select

# Edit the settings in a window of their own
trans-window settings

# List available windows
trans-window list-windows

# Translate text directly (no panel)
trans-window translate "Hallo Welt"
```

### Selection translation

Press the selection chord (`ctrl+alt+s` by default) with text selected in the
pane in front. The selection is read through the clipboard — as it stands by
default, copied out of the pane first when `TRANS_SELECT_COPY` says which chord
copies — and a window opens over the pane with the selection and its
translation together. Nothing is delivered: the text stays where it was
selected.

Where the terminal copies on a chord of its own, say which one:

```
TRANS_SELECT_COPY=ctrl+shift+c
```

The selection is then copied with that chord — clipboard saved, marked, chord
pressed, read, clipboard put back — instead of read as it stands. A selection
longer than 4000 characters is cut before it is sent.

### Windows daemon

`trans-windowd` waits for three chords and opens whichever window they name:

| Chord | Default | Opens |
| --- | --- | --- |
| `TRANS_HOTKEY` | `ctrl+alt+t` | The panel |
| `TRANS_SELECT_HOTKEY` | `ctrl+alt+s` | The selection window |
| `TRANS_CONFIG_HOTKEY` | `ctrl+alt+c` | The settings window |

```bash
# Start the daemon with a panel chord of your own
TRANS_HOTKEY=ctrl+shift+t trans-windowd
```

Each chord is claimed when the daemon starts, so one changed in the settings
window takes effect the next time it starts — the window says so when you save
one. Set a chord to `off` to leave it unclaimed. On layouts where `ctrl+alt`
types a character (AltGr on many of them), move the chords to `ctrl+shift`
before starting.

## Key bindings

### Panel

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

### Selection window

| Key | Action |
| --- | --- |
| `esc`, `ctrl+c` | Close |
| `ctrl+t` | Try the translation again after an error |
| `↑` `↓`, `k` `j` | Read the translation line by line |
| `pgup` `pgdn`, `space` | Read a page at a time |
| `g` `G`, `home` `end` | Jump to the start / the end |

### Settings window

| Key | Action |
| --- | --- |
| `↑` `↓`, `k` `j` | Move between settings |
| `←` `→` | Step a service or throw a flag |
| `enter` | Write a value in; the next service; a flag |
| `s`, `ctrl+s` | Save what changed |
| `esc` | Close — twice when something changed |
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
| `TRANS_HOTKEY` | `ctrl+alt+t` | Chord that opens the panel (`off` opens nothing) |
| `TRANS_SELECT_HOTKEY` | `ctrl+alt+s` | Chord that opens the selection window |
| `TRANS_CONFIG_HOTKEY` | `ctrl+alt+c` | Chord that opens the settings window |
| `TRANS_SELECT_COPY` | none | Chord pressed to copy the selection; without one the clipboard is read as it stands |
| `TRANS_READ_LANGUAGE` | `ZH` | Language read-mode results come back in |
| `TRANS_CAPTURE_KEYS` | `ctrl+shift+c` | Chord `--capture` presses to copy the selection |
| `TRANS_HISTORY` | `1` | `0` keeps no record of sent prompts |
| `TRANS_HISTORY_LIMIT` | `500` | Prompts kept before the oldest are dropped |
| `TRANS_TM` | `1` | `0` keeps translations for the session alone |
| `TRANS_TM_LIMIT` | `5000` | Sentences kept before the oldest are dropped |

### The settings window

`trans-window settings` — or the settings chord — opens every one of these
over the pane in front:

- Values are changed with `←` `→` or written in with `enter`, and `s` saves.
- What is saved goes into the `.env` in the config directory; every other line
  stays as it stands.
- A variable set in the environment wins over the file, so those rows carry an
  `env` mark: saving them does not change what they answer until it is taken
  out of the environment.
- Saving one of the three chords says to restart `trans-windowd`.
- The API key is shown as dots and never written out in the window.

### Services that need no key

```
TRANS_PROVIDER=gtranslate
```

- **`gtranslate`** — Google's public translate endpoint. Undocumented and can be rate limited.
- **`mymemory`** — Fallback with a smaller daily allowance.

## Local translation

Point the panel at a program instead of a service:

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

## Translation memory

While you write, the panel translates sentence by sentence and remembers what
each sentence translated to — and `tm.jsonl` keeps that memory between
sessions, so a sentence paid for once is not paid for again after a restart.
Each sentence is written as it is learned, in the order it was learned, and
`TRANS_TM_LIMIT` says how many are kept before the oldest are dropped.
`TRANS_TM=0` keeps the memory for the session alone. The file sits in the
state directory with the drafts and the record, written the same way — yours
alone, never half-written — and it holds your own prompts and their English.

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

## Read mode

The panel the other way round: English text goes in, your language comes back,
and nothing is delivered into the window — `ctrl+d` copies the result instead,
and `esc` closes.

```bash
trans-window open --read               # read what is on the clipboard
trans-window open --read --capture     # start from the selection in that window
```

Both are what a hotkey should invoke. Capture saves the clipboard, brings the
target window forward, presses `TRANS_CAPTURE_KEYS` to copy the selection, and
puts the clipboard back the way it was. See
[docs/read-mode.md](docs/read-mode.md) for the details, the terminals-that-treat-
the-chord-as-an-interrupt caveat, and the settings involved.

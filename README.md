# trans

[![Go](https://img.shields.io/badge/go-1.26.6-00ADD8?logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![ci](https://github.com/raincfhnj/trans/actions/workflows/ci.yml/badge.svg)](https://github.com/raincfhnj/trans/actions)

Write prompts in the language you think in and get them translated to English for your coding agent. The panel opens beside the pane you are in — your agent keeps its pane, the panel keeps its own.

![trans — write in your own language, send to the agent in English](docs/demo.gif)

## Why this exists

You think faster in your own language. But an agent answers in the language it was asked in, so your prompts end up in the replies, the comments, the commits and the docs. Translate the prompt and the drift has no source.

Side effect: your sentence and its English sit side by side, prompt after prompt. It rubs off.

## Features

- **Panel beside the agent** — `ctrl+alt+t` splits the terminal pane and opens the panel on the right; `ctrl+d` delivers the English into the pane beside and comes back for the next prompt
- **Popup instead, if you prefer** — `TRANS_PANEL_HOST=popup` is the floating window over the pane
- **Multiple translation services** — DeepL, Google, MyMemory, any OpenAI-compatible API, or a local command; the free ones need no key
- **Live translation** — the English appears as you write; code in backticks and fences is never sent
- **Read mode** — English in, your language back, `ctrl+d` copies instead of sending
- **Vim bindings**, kept drafts, sent-prompt history (`ctrl+g`), settings window (`ctrl+alt+c`), tray icon
- **Encrypted key** — the API key is wrapped with Windows DPAPI, never left in the file

## Installation

```powershell
winget install raincfhnj.trans
# or
scoop install trans
```

Until the first `v*` tag is published both manifests resolve to nothing — build from source:

```powershell
make windows   # both programs into bin\; put it on PATH
```

## Quick start

1. Start the daemon: `bin\trans-windowd.exe` — its icon appears in the notification area.
2. Over a terminal running anything — a shell, an agent — press `ctrl+alt+t`. The panel opens beside that pane.
3. Write in your language; `ctrl+d` translates and sends.

With no API key the panel is a plain draft box: the draft is handed on as you wrote it. To translate, pick a service in `%APPDATA%\trans\.env`:

```
TRANS_PROVIDER=gtranslate    # free, no key

# DeepL — a free tier key ends in :fx
TRANS_API_KEY=your-deepl-key

# any OpenAI-compatible API
TRANS_PROVIDER=openai
TRANS_API_KEY=sk-...
TRANS_ENDPOINT=https://api.deepseek.com/v1
TRANS_MODEL=deepseek-chat
```

## Usage

```bash
trans-window open                  # the panel beside the pane in front
trans-window open --review         # type the prompt in, leave the sending to you
trans-window open --target 0x1a2b  # deliver into a window of your choosing
trans-window tui                   # the panel in this terminal itself
trans-window tui --inline          #   drawn inline, the screen left alone
trans-window select                # translate what is selected in the window in front
trans-window open --read           # English in, your language back (ctrl+d copies)
trans-window open --read --capture #   starting from the selection in that window
trans-window settings              # the settings window
trans-window translate "Hallo Welt"  # translate without a panel
trans-window list-windows          # what can be delivered into, with handles
trans-window setup                 # the first-run doctor
```

The finished prompt goes to the pane the panel was split from; `--target` or
`TRANS_TARGET` names some other window instead. With no window at all the
prompt is copied to the clipboard and the panel says so.

## In the terminal

The panel chord — and `trans-window open` — open the panel **beside the pane in
front**: Windows Terminal splits that pane and the panel runs in the new one on
the right, whatever is running in the pane it was split from — an agent, a
shell, an editor. `esc` closes the panel with the pane it drew in.

`ctrl+d` crosses to the pane beside, pastes the English in and — when it sends —
comes back to the panel for the next prompt. A prompt that is only typed in
(`--review`) leaves the keyboard over there, where the last keystroke has to be
pressed.

`trans-window tui` typed by hand is the panel in the terminal it is typed into.
On a terminal that is not a Windows Terminal the chord types `trans-window tui`
into a prompt instead, and a pane with a program of its own in it is left alone
rather than written into.

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

Every setting is a line in the `.env` file or an environment variable; the
environment wins. The most used:

| Setting | Default | Meaning |
| --- | --- | --- |
| `TRANS_PROVIDER` | auto | Service: `deepl`, `google`, `gtranslate`, `openai`, `mymemory`, `cmd`, `dry-run`, `off` |
| `TRANS_API_KEY` | none | Credentials for the service (or `TRANS_<PROVIDER>_API_KEY`) |
| `TRANS_LANGUAGE` | `EN-US` | Target language |
| `TRANS_PANEL_HOST` | `terminal` | `popup` opens the panel as a window of its own instead |
| `TRANS_SUBMIT` | `1` | `0` types without sending |
| `TRANS_LIVE` | `1` | `0` translates only on send |
| `TRANS_VIM` | `0` | `1` enables vim bindings |
| `TRANS_THEME` | auto | Colour scheme: `ocean`, `forest`, `amber`, `mono` |
| `TRANS_TARGET` | none | Window the prompt is delivered into: a handle or a title |
| `TRANS_COMMAND` | none | A local program to translate with instead of a service |
| `TRANS_HOTKEY` | `ctrl+alt+t` | Panel chord (`off` opens nothing) |
| `TRANS_SELECT_HOTKEY` | `ctrl+alt+s` | Selection window chord |
| `TRANS_CONFIG_HOTKEY` | `ctrl+alt+c` | Settings window chord |
| `TRANS_TRAY` | `1` | `0` starts the daemon without a tray icon |

The full list — layout, chords, history, key storage, directories — is in
[docs/settings.md](docs/settings.md), which the settings window edits for you.

## More

- **Selection translation** — press `ctrl+alt+s` with text selected: the selection and its translation are drawn together and nothing is delivered. Terminals that copy on a chord of their own say which one in `TRANS_SELECT_COPY`.
- **Read mode** — `open --read` turns the panel around; see [docs/read-mode.md](docs/read-mode.md).
- **The tray** — open panel, settings, reload, start at logon; see [docs/tray.md](docs/tray.md).
- **The API key** — wrapped with DPAPI by default, or kept plain; see [docs/keys.md](docs/keys.md).
- **Local translation** — `TRANS_COMMAND=/path/to/translateLocally -m de-en-base`: draft to stdin, translation from stdout, nothing leaves the machine ([docs/local-translation.md](docs/local-translation.md)).
- **History** — what you wrote, what was sent, which window it went into; `ctrl+g` opens the record ([docs/history.md](docs/history.md)).
- **What leaves your machine** — the draft goes to the translation service, code in backticks and fenced blocks never does, and the key goes to the service only. A local translator keeps everything at home.

## Development

```bash
make tools     # the linters and the scanner, at the versions CI uses
make test      # the test suite, as CI runs it
make qa        # fmt, workflow lint, vet, lint, race, vuln
make cover     # the tests with a coverage report
make build     # build for Windows in bin/
make release   # the archives a release ships (docs/release.md)
```

`make qa` needs `make tools` once; `make release` additionally needs `zip` and
`sha256sum` in a POSIX shell. CI runs the same gate on every push and pull
request. See [docs/architecture.md](docs/architecture.md) for how the pieces
fit together.

## Credits

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss).

Reference: [wazum/herdr-polyglot](https://github.com/wazum/herdr-polyglot)

## License

[MIT](LICENSE)

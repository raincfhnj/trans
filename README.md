# trans

[![Go](https://img.shields.io/badge/go-1.26.6-00ADD8?logo=go)](https://go.dev/)
[![herdr](https://img.shields.io/badge/herdr-%E2%89%A5%200.8.0-6C3EF5)](https://herdr.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Write prompts in the language you think in and keep the conversation in English.
A [*herdr*](https://herdr.dev) plugin: press a key on any agent pane, an overlay
opens above it, you write, and the English translation lands in the agent's
input. It works with *Claude Code*, *Codex*, *opencode* and the rest, because
the prompt goes through *herdr* rather than into a particular tool.

This is a fork of [wazum/herdr-polyglot](https://github.com/wazum/herdr-polyglot)
with three more services — the free *Google* endpoint and *MyMemory* need no key
at all, and any *OpenAI*-compatible API can be used with one. The plugin's own id
here is `local.trans`.

## Why this exists

You think faster in your own language. But an agent answers in the language it was
asked in, so your prompts end up in the replies, the comments, the commits and the
docs — and a rule in `CLAUDE.md` or `AGENTS.md` cannot hold: the agent continues the
context it has, and every prompt you send makes more of that context your language.
Translate the prompt and the drift has no source.

Side effect: your sentence and its English sit side by side, prompt after prompt.
It rubs off.

## Quick start

```bash
git clone <this repository> trans && cd trans
herdr plugin link .
```

From a checkout the build needs the Go toolchain; `herdr plugin link .` builds
`bin/trans` through [`herdr/install.sh`](herdr/install.sh).

That is enough to use it: with no key the popup is a prompt box that hands what
you write to the agent, with the keys, the vim bindings and the resumed draft it
always had. Nothing is translated and nothing leaves the machine.

For the translation, put your key in the plugin's own config directory, readable
only by you — a plain redirect would leave it readable by everyone on the machine:

```bash
ENV_FILE="$(herdr plugin config-dir local.trans)/.env"
touch "$ENV_FILE" && chmod 600 "$ENV_FILE"
echo "HERDR_TRANS_API_KEY=your-deepl-key" >> "$ENV_FILE"
```

A [*DeepL* API key](https://www.deepl.com/pro-api) has a free tier. Free keys
end in `:fx`, and the plugin sends those to *DeepL*'s free host by itself.

Then bind a key in `~/.config/herdr/config.toml`:

```toml
[[keys.command]]
key = "prefix+t"
type = "plugin_action"
command = "local.trans.prompt"
description = "write a prompt in your own language"
```

For one gesture instead of two, let the terminal send the prefix for you. In
*Ghostty*:

```
keybind = super+p=text:\x02t
```

`\x02` is `ctrl+b`, the prefix, and `t` is the binding above. Reload with ⌘⇧, and
take a chord the terminal has free — ⌘G is find-next in *Ghostty*.

`t` as in translate — *herdr* already uses `prefix+p` for the previous tab.
There is a second action, `local.trans.compose`, which types the prompt into
the agent's input instead of sending it, and `ctrl+r` switches between the two
while you write, so one keybinding is enough.

## In the popup

| | |
| --- | --- |
| `alt+enter` | translate and hand the prompt over (`ctrl+d` does the same) |
| `enter` | a new line, because a prompt is often more than one |
| `ctrl+r` | switch between sending it and only filling the input |
| `ctrl+l` | turn live translation on or off for this prompt |
| `ctrl+t` | translate what is there now, or try again after a translation failed |
| `tab` | read the translation across the whole popup, and back |
| `ctrl+u` | throw the draft away, as it clears a line in a shell |
| `esc` | close — with vim bindings on, first to normal mode, then close |
| `ctrl+c` | close, always |

The header says which of the two will happen: `sends to agent` hands the prompt
over and the agent starts working, `fills the input` types it there and leaves
the last keystroke to you. When something does not work the footer says so,
`esc` takes the message away, and it goes by itself after a few seconds.

A sent prompt leaves the popup open and the box empty, ready for the next one;
only a prompt that was typed into the agent's input closes it, since there is
nothing left for the popup to do. What is typed is translated as you write.

Where a terminal draws an input method's pre-edit text is up to the terminal: it
follows the cursor. The panel keeps that cursor on the caret, writing the move
into its own output, so Chinese appears where it is being written — in a console
the console host draws and in Windows Terminal alike. Typing itself does not
depend on it either way.

## Settings

Every setting can be a line in the `.env` file or an environment variable, which
wins over the file. A value that is neither on (`1`, `true`, `yes`, `on`) nor
off (`0`, `false`, `no`, `off`) is refused rather than guessed at.

| Setting | Default | Meaning |
| --- | --- | --- |
| `HERDR_TRANS_API_KEY` | none | Credentials for the translation service |
| `HERDR_TRANS_PROVIDER` | picked from what is configured | Which service: `deepl`, `google`, `gtranslate`, `openai`, `mymemory`, `cmd`, `dry-run` or `off` |
| `HERDR_TRANS_MODEL` | the service's own | What a service that is a model should answer with, for `openai` |
| `HERDR_TRANS_PASTE_KEYS` | worked out from the terminal | The chord that pastes, for the panel on Windows: `ctrl+v`, `ctrl+shift+v` |
| `HERDR_TRANS_COMMAND` | none | A program that translates, for [local translation](docs/local-translation.md) |
| `HERDR_TRANS_LANGUAGE` | `EN-US` | Target language |
| `HERDR_TRANS_ENDPOINT` | the service's own | Override the service endpoint |
| `HERDR_TRANS_SUBMIT` | `1` | `0` types the prompt without sending it |
| `HERDR_TRANS_VIM` | `0` | `1` turns on the [vim bindings](docs/vim.md) |
| `HERDR_TRANS_LIVE` | `1` | `0` translates only on send, until `ctrl+l` |
| `HERDR_TRANS_CONFIRM` | `0` | `1` shows the English and waits for a second `alt+enter` |
| `HERDR_TRANS_KEEP_DRAFT` | `1` | `0` starts from an empty box instead of resuming |
| `HERDR_TRANS_MAX_DRAFT` | `2000` | Characters before the box says the draft is too long |
| `HERDR_TRANS_PULSE` | `1` | `0` stops the live circle from breathing |
| `HERDR_TRANS_LOGO` | `1` | `0` leaves the empty draft box unsigned |

The service is chosen by what you configured: a key means *DeepL*, a command means
that command, neither means `off` — the plain prompt box. Both at once is refused
rather than guessed at, and then `HERDR_TRANS_PROVIDER` decides. Asking for a
service that cannot be built is a case of its own: the popup says what is missing
and where it belongs, and delivers nothing, so an untranslated draft never reaches
the agent by accident. To keep keys for several services side by side, scope them by
name: `HERDR_TRANS_DEEPL_API_KEY`, `HERDR_TRANS_GOOGLE_API_KEY`.

*Google* takes the same language setting and needs the Cloud Translation API
enabled for the key. It reports no monthly count, so the header shows none with
it, and it has no unbilled context parameter — live translation still pays for
each sentence once, but translates each on its own.

### Services that need no key

Two of them are there for a first run, before anything has been signed up for:

```bash
echo 'HERDR_TRANS_PROVIDER=gtranslate' >> "$ENV_FILE"
```

- **`gtranslate`** is *Google*'s public translate endpoint. The same translation
  the web page gives, with no account and no count. Being an endpoint Google does
  not document for this, it can be rate limited, and a draft sent to it is a draft
  sent to *Google*.
- **`mymemory`** is the fallback when that endpoint refuses: no key either, a
  smaller daily allowance, and a translation that reads like a machine's. A draft
  longer than a few sentences is sent in pieces, and each piece is paid for by the
  service's own count rather than by yours.

### Any OpenAI-compatible API

```bash
echo 'HERDR_TRANS_PROVIDER=openai' >> "$ENV_FILE"
echo 'HERDR_TRANS_API_KEY=sk-...' >> "$ENV_FILE"
echo 'HERDR_TRANS_ENDPOINT=https://api.deepseek.com/v1' >> "$ENV_FILE"
echo 'HERDR_TRANS_MODEL=deepseek-chat' >> "$ENV_FILE"
```

Anything that speaks `POST /chat/completions` works: *OpenAI*, *DeepSeek*,
*Qwen*, *Moonshot*, a gateway, or a model on this machine behind the same shape.
A local endpoint may use `http` only when it is on `localhost`; anywhere else must
be `https`, because the key travels with every request. *DeepL* and this one are
told the sentence before the one being translated, so live translation stays cheap
with them.

## What else it does

**An unfinished prompt is kept.** Drafts are stored privately, one per pane, and
come back the next time you open the popup there, however it closed. A sent or
discarded draft is forgotten.

**Read the English first.** With `HERDR_TRANS_CONFIRM=1`, `alt+enter`
translates and shows the result, a second `alt+enter` delivers it, and `esc`
goes back to writing. It costs one translation, not two.

**Live translation.** The English follows your draft after a short pause. Each
sentence is paid for once, and a translation you have already read is delivered
as it stands, so writing costs little more than sending —
[how that works](docs/live-translation.md). It starts on; `HERDR_TRANS_LIVE=0`
keeps it off until `ctrl+l` asks for it. A draft that came back from an earlier
session and text you paste in stay untranslated until then, since neither is
something you asked to have translated.

**Translation on your own machine.** `HERDR_TRANS_COMMAND` points the plugin at
a program instead of a service, so a draft need not leave the machine at all —
[how to set that up](docs/local-translation.md).

**Code is not translated.** A translation service rewrites code as if it were
prose: it renames identifiers, translates comments and string literals, and
reformats indentation. So anything in backticks or a fenced ``` block is taken out
of the draft before it is sent, and put back exactly as it was — which also means
it never leaves your machine and costs nothing to translate.

**Vim bindings.** `HERDR_TRANS_VIM=1` makes the draft box modal, with the
motions, edits and counts that make sense inside a text box —
[the full list](docs/vim.md).

## On a native terminal in Windows

There is no *herdr* on Windows, so nothing hosts a popup there — the panel brings
its own window. It opens over the window the prompt is written for, keeps the
draft box, the keys, the translation and the resumed draft of the plugin, and
pastes the translated prompt into that window, with or without the return.

```bash
make windows                      # bin/trans-window.exe, bin/trans-windowd.exe
trans-window list-windows      # what can be opened over, with the handles
trans-window open              # opens the panel over the window in front
trans-window open --review     # ...and only types the prompt in
trans-window open --target 0x1a2b3c
trans-window translate "text"  # asks the configured service, without a panel
```

`trans-windowd` waits for `HERDR_TRANS_HOTKEY` (`ctrl+alt+t`) and opens the
panel over whatever is in front at that moment, so one key is enough from any
pane. It has no window of its own and can sit in a logon entry; what it has to say
goes to `%LOCALAPPDATA%\trans\windowd.log`, and the panel's own log sits next
to it.

Settings are the ones above, read from the same `HERDR_TRANS_*` environment
variables or from `%APPDATA%\trans\.env`, which is written with the settings
commented out the first time the panel opens. Without a key, the free services
are the ones to reach for:

```
HERDR_TRANS_PROVIDER=mymemory
```

A draft is kept per window, keyed by its title, so the pane you wrote in is the
pane you find it in again.

In *Windows Terminal* one key does the whole thing: start `trans-windowd` once
(a shortcut in `shell:startup` keeps it there), press `ctrl+alt+t` while working
in a pane, write, and the English lands in that pane. *Windows Terminal* has
`alt+enter` bound to fullscreen of its own, so the send key there is `ctrl+d`,
which sends the same way; removing the `toggleFullscreen` binding brings
`alt+enter` back. `ctrl+r` switches between sending and only filling the input,
before the prompt goes anywhere.

*Windows Terminal* draws a window of its own and cannot be made frameless, so the
panel is a terminal window of that program, placed over the pane it belongs to.
Where a console is still drawn by the console host itself, the panel takes the
frame off and is a proper popup.

A prompt is delivered by pasting it and pressing return, and what a terminal
takes for a paste is not the same everywhere: *Windows Terminal* pastes on
`ctrl+shift+v`, the console host on `ctrl+v`. The panel looks at what is drawing
its own console and takes that chord; `HERDR_TRANS_PASTE_KEYS` says otherwise
if a terminal was configured against its default.

## What leaves your machine

The draft goes to the translation service, so treat it the way you treat
anything you paste into a web translator: prompts for a coding agent carry file
paths, code and occasionally a secret, and in live mode the draft goes out again
after every pause in typing.

Code does not leave either: backticked spans and fenced blocks are held back and
restored afterwards, so a pasted stack trace or a file path is not sent anywhere.

Nothing else leaves. The API key goes to the translation service only — never to
the agent, the *herdr* socket, a command line or a child process. Translated
text is stripped of control characters before it is typed into a pane, so
neither a line break nor an escape sequence can reach the agent's terminal.
A [translator on the machine](docs/local-translation.md) keeps a draft off the
network entirely.

## Development

```bash
make qa       # formatting, linting, race tests, vulnerability scan
make build
make cross    # the four binaries herdr's platforms need
herdr plugin link .
```

The overlay names palette slots rather than fixed colours, so it takes on
whatever *herdr* theme is active, including a light one.
[How the pieces fit together](docs/architecture.md), including how to add
another translation service.

## Credits

A fork of [wazum/herdr-polyglot](https://github.com/wazum/herdr-polyglot),
created with ♥ by [Wolfgang Klinger](https://wolfgang-klinger.dev/). The added
services live in their own packages and change nothing about the design it was
built on.

Built on [*herdr*](https://herdr.dev),
[*Bubble Tea*](https://github.com/charmbracelet/bubbletea) and
[*Lip Gloss*](https://github.com/charmbracelet/lipgloss).

## License

[MIT](LICENSE).

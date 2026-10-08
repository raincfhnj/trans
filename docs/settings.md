# The settings

Every setting is a line in the `.env` file in the config directory or an
environment variable — except `TRANS_CONFIG_DIR`, which the environment alone
decides, the file being only found once its directory is known. The settings
window is the friendly way to edit most of them; the rest are written into
`.env` by hand. This page says what each one does and how to use it.

## Opening the settings window

- Press the settings chord (`ctrl+alt+c` by default), or
- run `trans-window settings`, or
- choose *Settings…* from the tray icon's right-click menu.

The window opens over the pane in front, the same as the panel does. It shows a
row per setting: the name on the left, the value on the right, and an `env` mark
when the environment is what answers for that row rather than the file.

## Using the window

| Key | Action |
| --- | --- |
| `↑` `↓`, `k` `j` | Move between settings. |
| `←` `→` | Step a service, or throw a flag on and off. |
| `enter` | Write a value in; on a service row, move to the next service; on a flag, switch it. |
| `s`, `ctrl+s` | Save what changed into the `.env` file. |
| `esc` | Close. When something changed, a second `esc` confirms. |
| `ctrl+c` | Close always. |

What is saved goes into the `.env` in the config directory; every other line in
that file stays as it stands. A variable set in the environment wins over the
file, so those rows carry an `env` mark: saving them does not change what they
answer until the variable is taken out of the environment.

Saving one of the three chords writes a notice that the daemon has to claim
them again — it says to restart `trans-windowd`, and the tray's *Reload
settings* does the same in place.

The API key is shown as dots and never written out in the window.

## The settings in the window

### Translation

| Setting | Default | What it does and how to use it |
| --- | --- | --- |
| **service** | `auto` | Which translation service the panel asks. Step with `←` `→` through `auto`, `deepl`, `google`, `gtranslate`, `openai`, `mymemory`, `cmd`, `dry-run`, `off`. `auto` picks the best configured one; `off` turns translation off and hands your draft on as you wrote it. Free services (`gtranslate`, `mymemory`) need no key. |
| **api key** | none | The credentials for the service that needs them. Type it with `enter` and save. Shown as dots. Leave empty for the free services. Under `TRANS_KEYS=dpapi` (the default) the key is encrypted and kept in `secrets.json`, not in `.env`. |
| **endpoint** | the service's own | Where to send OpenAI-compatible requests. Set it for a self-hosted or alternate API, e.g. `https://api.deepseek.com/v1`. Leave empty to use the service's own endpoint. |
| **model** | the service's own | The model name for OpenAI-compatible APIs, e.g. `deepseek-chat`. Leave empty for the service's default. |
| **language** | `EN-US` | What prompts are translated into. Write the code your agent should receive, e.g. `EN-US`, `EN-GB`, `DE`. |
| **command** | none | A local program to translate with instead of a service. The draft goes to its stdin and the translation comes from its stdout. Nothing leaves your machine. e.g. a path to `translateLocally`. |

### The panel

| Setting | Default | What it does and how to use it |
| --- | --- | --- |
| **submit** | `on` | `on` sends the finished prompt into the window (and to the agent); `off` only types it in so you can review before sending. |
| **live** | `on` | `on` translates as you write, in the box beside the draft (`ctrl+l` toggles it while the panel is open); `off` translates only when you send. |
| **vim** | `off` | `on` turns on vim bindings for the draft box (modal editing). |
| **confirm** | `off` | `on` shows the English before anything is sent, so you can read it and confirm with `ctrl+d`. |
| **keep draft** | `on` | `on` keeps your unfinished draft between sessions; `off` starts every panel with an empty box. |
| **theme** | `auto` | Which colour scheme all three windows draw in (`TRANS_THEME` in `.env`). Step with `←` `→` through `auto`, `ocean`, `forest`, `amber` and `mono`. `auto` leaves the colours to the terminal; a name in `.env` the program has never heard of draws with the default rather than failing. This window repaints as you step — the panel and the selection window take it up the next time they open. |
| **draft rows** | `6` | How many rows the panel's draft box asks the terminal for, from `4` to `16` (`TRANS_DRAFT_ROWS` in `.env`). The arrows step it by one and `enter` writes a number; a value outside the range is refused before anything is saved, and a hand-written `.env` line outside it is refused when the settings are read. The popup grows with it: the translation box keeps its five rows, so a draft that asks for more makes the whole popup that many rows taller. |
| **panel width** | `110` | How many columns the panel popup asks for, from `60` to `180` (`TRANS_PANEL_WIDTH` in `.env`). Stepped with `←` `→` by one and written in with `enter`, checked against both ends before it is saved; a hand-written `.env` line outside them is refused when the settings are read. |

A theme names slots of the terminal's palette rather than colours of its own:
`auto`, `ocean`, `forest`, `amber` and `mono` only point the accent and the
bright colours at slots 0-15, so no popup carries a shade the terminal did not
choose and every window follows whichever theme the terminal is running.

### The chords

| Setting | Default | What it does and how to use it |
| --- | --- | --- |
| **paste keys** | automatic | Which chord the panel presses to paste the finished prompt into the window. `ctrl+v` for the console's own window, `ctrl+shift+v` for a terminal program. Set it by hand if yours needs a different one. |
| **select copy** | none | The chord pressed to copy a selection before it is read for translation. Without one, the clipboard is read as it stands. Set it when your terminal copies on a chord of its own, e.g. `ctrl+shift+c`. |
| **panel hotkey** | `ctrl+alt+t` | The chord that opens the panel. `off` leaves it unclaimed. |
| **selection hotkey** | `ctrl+alt+s` | The chord that opens the selection window (translate what is selected). |
| **settings hotkey** | `ctrl+alt+c` | The chord that opens this settings window. |

On layouts where `ctrl+alt` types a character (AltGr on many of them), move the
chords to `ctrl+shift` before starting the daemon.

## Settings only in the `.env` file

These are not in the window. Write them as lines in `.env` or set them in the
environment.

| Setting | Default | What it does |
| --- | --- | --- |
| `TRANS_MAX_DRAFT` | `2000` | Characters in the draft before the panel warns you it is long. |
| `TRANS_PULSE` | `1` | `0` stops the live circle animation. |
| `TRANS_LOGO` | `1` | `0` hides the mark the panel signs an empty draft box with. |
| `TRANS_HISTORY` | `1` | `0` keeps no record of sent prompts. |
| `TRANS_HISTORY_LIMIT` | `500` | How many sent prompts are kept before the oldest are dropped. |
| `TRANS_READ_LANGUAGE` | `ZH` | The language read-mode results come back in (kept apart from the language prompts are translated into). |
| `TRANS_CAPTURE_KEYS` | `ctrl+shift+c` | The chord `open --read --capture` presses to copy the selection. |
| `TRANS_KEYS` | `dpapi` | `plain` leaves the API key in the `.env` file instead of wrapping it with the Windows Data Protection API. |
| `TRANS_TRAY` | `1` | `0` starts the daemon with no tray icon. |
| `TRANS_CONFIG_DIR` | `%APPDATA%\trans` | Where the `.env` and `secrets.json` live. Environment only — the file is found *through* this directory, so a line inside it cannot move it. |
| `TRANS_STATE_DIR` | `%LOCALAPPDATA%\trans\state` | Where an unfinished draft and the record of sent prompts are kept between sessions. |
| `TRANS_TARGET` | none | The window the panel opens over when nothing is named in front of it: a handle (`0x1a2b`) or a title. Read by the panel rather than edited anywhere. |

## Where the settings live

- **`.env`** in the config directory (`%APPDATA%\trans\.env` on Windows): every
  setting, one per line, `NAME=value`.
- **`secrets.json`** beside it: the encrypted API key, when `TRANS_KEYS=dpapi`.
- **The environment**: any variable set here wins over the file, and the window
  marks those rows `env`.

See [keys.md](keys.md) for where the API key is kept, and the README's
*Settings* table for the same list in brief.

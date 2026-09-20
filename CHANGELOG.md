# Changelog

All notable changes to this project are written down here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and
the versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.5.0] - 2026-09-20

### Changed

- **Breaking**: Renamed all environment variables from `HERDR_TRANS_*` to `TRANS_*`
- Removed herdr plugin integration — this is now a standalone Windows application
- Removed `cmd/trans` (herdr plugin entry point)
- Removed `internal/herdr` package
- Removed `herdr-plugin.toml` and `herdr/` directory
- Updated `Makefile` to build Windows binaries directly
- Rewrote documentation for standalone usage

### Added

- `TRANS_HOTKEY` setting for the daemon (default: `ctrl+alt+t`)
- `TRANS_CONFIG_DIR` and `TRANS_STATE_DIR` for explicit directory configuration

### Removed

- herdr plugin support
- `cmd/trans` binary (was the herdr plugin)
- `internal/herdr` package (socket, exec runner, targets)
- `HERDR_BIN_PATH` setting

## [0.4.0] - 2026-09-14

### Added

- A prompt that was only typed into the agent's input no longer closes the
  panel: the box is emptied and waits for the next prompt, the same as one that
  was sent. Closing is what esc is for.
- The panel keeps the terminal's cursor on the caret, so an input method's
  pre-edit text appears where the writing is — in a console the console host
  draws and in Windows Terminal alike.
- The footer of the panel on Windows names the key that works there: Windows
  Terminal keeps alt+enter for itself, so the panel says ctrl+d send instead of
  naming a chord that only opens the window full screen.
- The panel stays open after a prompt was sent: the box is emptied and waits for
  the next prompt, and only a prompt that was typed into the agent's input closes
  the panel, since there is nothing left for it to do.
- `HERDR_TRANS_PASTE_KEYS` says which chord pastes, for the panel on Windows:
  *Windows Terminal* takes `ctrl+shift+v` and the console host `ctrl+v`, and the
  panel works out which of them it is drawing in.
- *MyMemory* and *Google*'s public endpoint as services that need no key at all
  (`HERDR_TRANS_PROVIDER=mymemory`, `gtranslate`), so the popup translates
  before anything has been signed up for.
- Any *OpenAI*-compatible API as a service (`HERDR_TRANS_PROVIDER=openai`),
  with `HERDR_TRANS_ENDPOINT` for a gateway or a model on this machine and
  `HERDR_TRANS_MODEL` for what it should answer with.
- The panel also runs on a native terminal in Windows: `trans-window` opens
  the same draft box over the window the prompt is written for and pastes the
  translation into it, with `trans-windowd` opening it on a hotkey from
  wherever the author is working.
- A translation service can be a program on your own machine:
  `HERDR_TRANS_COMMAND` names a command line, the draft is written to its input
  and its answer is the translation.
- The plugin works with no API key at all: the popup opens as a prompt box that
  hands what you write to the agent.
- `tab` flips between writing and reading.
- `ctrl+t` translates the draft as it stands.
- The panel being written in or read is drawn with the accent border.
- How far through a panel you are is written into its bottom border.

### Changed

- The panel opens wider: 110 columns instead of 90.
- Live translation is on when the popup opens.
- The project is called *trans*: the module, the binaries, the plugin id and the
  settings prefix.

### Fixed

- A service that times out or cannot be reached says so in a sentence.
- Reading a translation that failed shows what went wrong.
- The boxes are given their rows again when a translation arrives.
- The scrollbar says where the view actually is.
- A draft that comes back opens at its beginning.
- The draft can be walked through when it holds more than it shows.
- Pasting a long draft no longer leaves the view at the top.
- Nothing is drawn wider than the pane.
- The translation beside the draft shows its beginning and ends in `…`.
- Text no longer wraps twice.
- The popup keeps its shape whatever is written in it.

## [0.3.0] - 2026-08-18

### Added

- The plugin signs the empty draft box with a small braille mark.

### Changed

- A delivered prompt keeps its line breaks.
- Code is no longer translated.

## [0.2.0] - 2026-08-18

### Added

- *Google Cloud Translation* as a second service.

## [0.1.0] - 2026-08-18

First release.

### Added

- An overlay over any agent pane, opened by a keybinding.
- Two ways to deliver it: sent to the agent, or typed into its input.
- *DeepL* as the translation service.
- Live translation.
- Confirmation mode.
- Unfinished drafts kept privately.
- Vim bindings in the draft box.
- The month's character count in the header.
- Settings from the plugin's `.env` or the environment.
- Prebuilt binaries for macOS and Linux.

[0.5.0]: https://github.com/raincfhnj/trans/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/raincfhnj/trans/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/raincfhnj/trans/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/raincfhnj/trans/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/raincfhnj/trans/releases/tag/v0.1.0

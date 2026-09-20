# Changelog

All notable changes to this project are written down here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and
the versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.4.0] - 2026-09-14

### Added

- A prompt that was only typed into the agent's input no longer closes the
  panel: the box is emptied and waits for the next prompt, the same as one that
  was sent. Closing is what esc is for.
- The panel keeps the terminal's cursor on the caret, so an input method's
  pre-edit text appears where the writing is — in a console the console host
  draws and in Windows Terminal alike. The move is written into the panel's own
  output, which a terminal program takes like any other drawing.
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
  before anything has been signed up for. The public endpoint is undocumented by
  Google and can be rate limited; *MyMemory* has a small daily allowance and is
  there for when it is.
- Any *OpenAI*-compatible API as a service (`HERDR_TRANS_PROVIDER=openai`),
  with `HERDR_TRANS_ENDPOINT` for a gateway or a model on this machine and
  `HERDR_TRANS_MODEL` for what it should answer with. It is told the sentence
  before the one being translated, the way *DeepL* is, so live translation stays
  cheap with it too.
- The panel also runs on a native terminal in Windows: `trans-window` opens
  the same draft box over the window the prompt is written for and pastes the
  translation into it, with `trans-windowd` opening it on a hotkey from
  wherever the author is working. There is no herdr there to host a popup, so
  the panel brings its own window.
- A translation service can be a program on your own machine:
  `HERDR_TRANS_COMMAND` names a command line, the draft is written to its input
  and its answer is the translation. No key, no account, nothing over the network —
  [translateLocally](https://translatelocally.com) is quick enough for live
  translation. See [docs/local-translation.md](docs/local-translation.md).
- The plugin works with no API key at all: the popup opens as a prompt box that
  hands what you write to the agent, with every key, the vim bindings and the
  resumed draft. Install it, bind it, use it — a key adds the translation later.
  `HERDR_TRANS_PROVIDER=off` asks for that box with a key configured.
- `tab` flips between writing and reading. Reading gives the translation the whole
  popup, scrolled with `↑ ↓ PageUp PageDown` — or `j k g G` with vim bindings on —
  and shows how far down it you are. Any letter, `tab` or `esc` goes back to the
  draft, so it cannot be got stuck in.
- `ctrl+t` translates the draft as it stands: the way to try again when a
  translation did not arrive, and the way to read the English at all with live
  translation off, without sending anything.
- The panel being written in or read is drawn with the accent border, the other in
  the frame's grey, so it is clear which one has the keys.
- How far through a panel you are is written into its bottom border rather than
  the footer.

### Changed

- The panel opens wider: 110 columns instead of 90, so a sentence of the
  translation can be read beside the draft as it stands.
- Live translation is on when the popup opens, since a prompt is usually written
  to be translated; `HERDR_TRANS_LIVE=0` keeps it off until `ctrl+l` asks for it.
  A draft that came back from an earlier session is still held back.
- The project is called *trans*: the module, the binaries (`trans`,
  `trans-window`, `trans-windowd`), the plugin id (`local.trans`) and the
  settings prefix (`HERDR_TRANS_`). The fork of *polyglot* keeps its history and
  its credit; the old `HERDR_POLYGLOT_` names are gone rather than aliased.
- Which service a draft goes through is decided in one place,
  `internal/service`, where both the popup and a test can ask for it, rather than
  inside the command that opens the overlay.
- The project is called *trans*: the module, the binaries (`trans`,
  `trans-window`, `trans-windowd`), the plugin id (`local.trans`) and the
  settings prefix (`HERDR_TRANS_`). A fork of *polyglot* keeps its history and
  its credit, and the old `HERDR_POLYGLOT_` names are gone rather than aliased.
- A service that is a program on the machine is run through the system's own
  shell: `sh` where there is one, `cmd.exe` where the plugin is built on Windows.
- A missing or unusable key is said in the popup, naming the file it belongs in,
  instead of the plugin refusing to open.

### Fixed

- A service that times out or cannot be reached says so in a sentence, instead of
  showing a Go error with a URL and a client timeout in it.
- Reading a translation that failed shows what went wrong, rather than claiming
  nothing has been translated yet.
- The boxes are given their rows again when a translation arrives or a
  confirmation opens, so the popup does not outgrow its pane until live
  translation is toggled.
- The scrollbar says where the view actually is. It was worked out from the cursor
  on the assumption that the last row was on screen, so it claimed the top while
  rows were hidden above.
- A draft that comes back opens at its beginning, where it can be read, instead of
  scrolled to its end.
- The draft can be walked through when it holds more than it shows: the arrows
  move by row, and `gj`/`gk` do the same with vim bindings on, where `j`/`k` keep
  vim's meaning of a whole line.
- Pasting a long draft no longer leaves the view at the top with the cursor out of
  sight below.
- Nothing is drawn wider than the pane. A popup narrower than 34 columns used to
  be drawn at 34 and wrap every line, stacking frame on frame while scrolling.
- The translation beside the draft shows its beginning and ends in `…` when there
  is more, instead of a scrollbar that could not be used from there. `tab` reads
  the rest, and the footer says so while there is more to read.
- Text no longer wraps twice. The draft box was two columns wider than what it
  showed, so a line was wrapped once by the text area and again by the box, which
  dropped words onto lines of their own.
- The popup keeps its shape whatever is written in it. A long translation used to
  grow its box and push the footer out of the pane; both boxes now scroll, with a
  bar showing how much is out of view, and the keys drop off the footer one at a
  time when a pane is too narrow for them.

## [0.3.0] - 2026-08-18

### Added

- The plugin signs the empty draft box with a small braille mark, in the corner
  furthest from the writing. It goes as soon as there is anything in the box, and
  `HERDR_TRANS_LOGO=0` leaves the box bare.

### Changed

- A delivered prompt keeps its line breaks instead of being flattened onto one
  line, so a protected code block arrives as a code block and a `//` comment no
  longer swallows the rest of it. Tabs arrive as four spaces; escape sequences are
  still stripped.
- Code is no longer translated. Backticked spans and fenced blocks are taken out
  of the draft before it is sent and put back exactly as they were, so identifiers
  keep their names, comments and string literals stay as written, and indentation
  survives. The code never reaches the service, so it costs nothing to translate
  and stays on your machine.

## [0.2.0] - 2026-08-18

### Added

- *Google Cloud Translation* as a second service, chosen with
  `HERDR_TRANS_PROVIDER=google` and a key in `HERDR_TRANS_GOOGLE_API_KEY`.
  The language setting is shared: `EN-US` reaches *DeepL* as it stands and
  *Google* as `en`, and a region is kept only where the region is the point.

## [0.1.0] - 2026-08-18

First release. Settings, keys and behaviour may still change while the plugin
meets other people's terminals — that is what the 0.x is for.

### Added

- An overlay over any agent pane, opened by a keybinding. Write the prompt in
  your own language; `alt+enter` translates it and hands it over.
- Two ways to deliver it: sent to the agent, or typed into its input for you to
  send. `ctrl+r` switches between them while you write, and the header says which
  it will be.
- *DeepL* as the translation service, with `dry-run` for checking the wiring
  without a key. The service sits behind an interface, so another one is a
  package away.
- Live translation, off by default: the English follows the draft as you write.
  Each sentence is paid for once, and a translation you have already read is
  delivered without being translated again. It stays off for a draft that came
  back from an earlier session and for pasted text until `ctrl+l` asks for it.
- Confirmation mode: see the English and press `alt+enter` again to deliver it.
- Unfinished drafts kept privately, one per pane, and restored the next time the
  popup opens — however it was closed, including *herdr* closing it.
- Vim bindings in the draft box, off by default: the motions, edits, counts and
  registers that make sense inside a text box.
- The month's character count in the header, when the service reports it, and a
  warning when a draft grows past prompt size.
- Settings from the plugin's `.env` or the environment. A value that is neither
  on nor off is refused rather than guessed at.
- Prebuilt binaries for macOS and Linux on arm64 and amd64, published with
  checksums and build provenance. Installing needs no Go toolchain.

[0.4.0]: https://github.com/raincfhnj/trans/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/raincfhnj/trans/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/raincfhnj/trans/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/raincfhnj/trans/releases/tag/v0.1.0

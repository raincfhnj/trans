# How it works

The daemon, `trans-windowd`, waits for three chords and hands off to
`trans-window select`, `trans-window open` or `trans-window settings`
according to which one was pressed. Each of them opens a floating popup over
the window in front, so everything lands where the key was pressed.
`trans-window open` is the same opening from a command line, with `--target` to
name another window.

The popup itself is a second process, started as `trans-window popup` with the
window it draws named on its own command line — `--selection` and `--settings`
for the two smaller windows, nothing for the panel, and `--read`, `--capture`,
`--review`, `--send` or `--probe` for what was asked of it. Switches travel
there rather than in the environment because a command line can be checked and
refused before anything is drawn, and because the process that reads it is the
only one that can be sure who wrote it. What rides in the environment instead is
what is
a setting or a piece of text rather than a switch: the pane to open over, which
the parent resolved while it was still the window in front, and the selection
one of the windows was opened for.

The panel is the draft box: on `alt+enter` the draft goes to a translation
service and the result is handed back to the same window through `wintarget`:
the window is brought forward and waited for, the text is put on the clipboard,
the paste chord presses it in, and — once the pane has had its moment to read
it — return is sent to hand it over. The review action stops before the
return, leaving that keystroke to you.

The selection window only reads. The process the chord woke takes the
selection out of the pane — the clipboard read as it stands by default, and
with `TRANS_SELECT_COPY` set the whole clipboard kept to one side, the pane
asked to copy, the result read and the clipboard put back — and passes the text
to its window in the environment; there the service is asked and the answer is
drawn beside the selection. What is kept is every format the clipboard holds and
not only its text, because what the chord is about to write over is whatever the
author copied last; a clipboard whose only content cannot be copied back is
refused rather than written over, since a read that did not happen costs a read
while the author's screenshot is gone for good. The settings window writes: it
changes the rows it is asked to change and rewrites the `.env` in place, leaving
every other line alone. Neither of them delivers anything.

## The pieces

| Package | Holds |
| --- | --- |
| `promptflow` | The use case: translate a draft, deliver a prompt. Owns nothing about terminals. |
| `translation` | The `Translator` and `Provider` ports, the registry of services, the sentence-level cache used for previews, and the decorator that keeps code out of a translation. |
| `deepl`, `google`, `gtranslate`, `openai`, `mymemory`, `command` | Services behind those ports: three hosts, one API any host can speak, and one program on the machine. |
| `service` | Which of them a draft goes through, and what to say when it cannot be built. The panel and `trans-window translate` both ask it, so neither answers the question on its own. |
| `overlay` | The *Bubble Tea* program: the draft box, the header and footer, the keys. |
| `selection` | The window that draws a selection and its translation together. It translates and scrolls; it never delivers. |
| `settings` | The window that edits the settings: one row each for the service, its options, the panel's behaviour and the three chords, saved through `config.Save`. |
| `frame` | What the three windows share: the table of built-in themes and the palette they name, the boxes with their labels, the wrapped rows, and the scroll bar. The overlay had drawn its own copies of these before, which is how the two drifted. |
| `vimarea` | A text area with modal editing, used by the overlay. |
| `config` | Settings from the environment and the `.env` in the config directory — read by `Load`, prepared and kept by `Prepare`, and rewritten line by line by `Save`. |
| `draft`, `history`, `atomicfile` | An unfinished prompt on disk, one file per window; the record of prompts already delivered; and the write the draft and the settings file go through: a fresh file for its owner alone, moved into place so nothing is ever read half-written. The record appends to its own file as prompts go out, and only takes that rewrite when the oldest are trimmed. |
| `win32`, `wintarget`, `winlog` | The same windows on Windows: the chords the daemon claims, the selection read out of the pane, the window each popup opens over, the paste that delivers a prompt into it, and the log a program without a console has to write to. Each waits on a signal where Windows offers one — the clipboard sequence number, the window in front, the target's input queue — and only falls back to a named delay where none exists. |
| `httpapi` | The one HTTP transport the five services share: timeouts, response caps, a redirect guard that keeps a key from leaving https, retry with backoff and `Retry-After`, and the error kinds `translation.Trouble` turns into sentences. |
| `secrets` | The provider key at rest, wrapped with Windows DPAPI, in a file of its own beside the `.env`; `config.UpgradeSecrets` moves a plaintext key into it and `ResolveKey` reads it back for whoever asked. |
| `tray` | The notification-area icon the daemon carries: its own hidden window and message loop on a thread of its own, and the menu that opens the panel, the settings, a reload, the logon entry and the end of the program. |
| `setup` | The first-run doctor: what a working installation needs — the toolchain, the settings file, the service and its key, the programs on `PATH`, a speech voice — reported as lines or as one JSON object. |

`cmd/trans-window` is the composition root for all three windows: it reads the
settings, resolves the key from wherever it is kept, asks `service` for the one
that was chosen, wires the flow to its targets and starts the window its
command line named — the panel, the selection, or the settings. It also carries
`setup`, which is the same reading of the settings with the answer printed
instead.
`cmd/trans-windowd` is the daemon beside it, claiming the chords the settings
name and handing off to the subcommand each one names, with the tray beside the
chord loop: the chords belong to the thread that claimed them, so the tray — on
a thread of its own — only posts to that one, and a reload is the daemon's own
work rather than the tray's. Nothing below them knows which service is in use or
how the popup was opened.

`promptflow` owns the ports it needs — `Translator`, `Target`, `UsageReporter` —
and imports no adapter package, not even `translation`. Where the two
vocabularies have to meet, they meet in `internal/service`: it is the only
place that knows both a `translation.Usage` and a `promptflow.Usage` exist, and
it adapts one to the other, so `cmd/trans-window` asks it for a
`promptflow.UsageReporter` and never sees both itself. `promptflow.New`
assumes the dependencies it is handed are real; a composition root is its only
caller, and a nil would fail on the first keystroke.

Previews and sends use different translators. A send is one shot and goes to the
service directly; a preview goes through the sentence cache, because writing
means translating the same draft again and again. Both are set up whether live
translation starts on or off, since `ctrl+l` can turn it on at any time.

The draft is stored when the program ends, whichever way it ended: closing the
panel's window hangs up on it, and `ctrl+c` is caught as a signal, so the last
word on the draft cannot be a keystroke.

## Another translation service

*DeepL* is one implementation of a small interface, not a dependency of the design:

```go
type Provider interface {
	Name() string
	New(Options) (Translator, error)
}

type Translator interface {
	Translate(ctx context.Context, draft string) (string, error)
}
```

A service that can be told what came before a sentence — and does not bill for
it — also implements `ContextualTranslator`, which is what makes live
translation cheap:

```go
type ContextualTranslator interface {
	TranslateWithContext(ctx context.Context, text, preceding string) (string, error)
}
```

`translation.Options` is a superset — `APIKey`, `TargetLanguage`, `Endpoint`,
and the two wider ones, `Model` for a service that is a model rather than a
translator and `Command` for one that is a program on the machine — and each
service takes what applies and ignores the rest. Every field there is one more
than the providers that do not need it carry, which is the price of keeping
the interface itself at `New(Options)`. Something narrower still — a project
id, a region — is better parsed in the composition root and handed to the one
service that asked for it, rather than added to a struct every service has to
look at.

Add a package that implements `Provider`, register it in `Registry()` in
`internal/service/service.go`, and it becomes selectable through
`TRANS_PROVIDER`. The first registered service is the default. A
service that reports what it has spent can implement `UsageReporter`, and the
header shows the count. One that can be told the sentence before the one it is
translating implements `ContextualTranslator`, which is what makes live
translation cheap with it.

The list is written out in one place on purpose, and there are two things it
does not do. It does not register services by side effect from their own
`init()` — a program that links a package to read one of its constants would
suddenly offer its service too — and it does not load services from outside the
binary. A build therefore carries every service it knows, and there is no way
to offer a seventh without rebuilding. For a panel that ships as one `.exe` per
machine that is the right trade: a plugin boundary would add a versioned
interface, a load order and a failure mode, in exchange for a smaller binary
nobody asked for. If it ever stops being right — a service that must be
installed separately, or a build that must not link the network — the way in is
a `Register` that takes a factory and a build tag per service, not a change to
the ports.

## Working on it

```bash
make qa     # formatting, workflow lint, vet, lint, race tests, vulnerability scan
make build
```

Tests are written from the outside in and named as sentences about behaviour.
The overlay, the selection window and the settings window are driven through
`teatest`, which means their tests type keys and read frames rather than
reaching into the model.

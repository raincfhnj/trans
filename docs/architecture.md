# How it works

The daemon, `trans-windowd`, waits for three chords and hands off to
`trans-window select`, `trans-window open` or `trans-window settings`
according to which one was pressed. Each of them opens a floating popup over
the window in front, passing the target window handle along in the environment,
so everything lands where the key was pressed.
`trans-window open` is the same opening from a command line, with `--target` to
name another window.

The panel is the draft box: on `alt+enter` the draft goes to a translation
service and the result is handed back to the same window through `wintarget`:
the text is put on the clipboard, the window is brought forward and pasted
into, and return is pressed to send it. The review action stops before the
return, leaving that keystroke to you.

The selection window only reads. The process the chord woke takes the
selection out of the pane — the clipboard read as it stands by default, and
with `TRANS_SELECT_COPY` set the clipboard saved, the pane asked to copy, the
result read and the clipboard put back — and passes the text to its window in
the environment; there the service is asked and the answer is drawn beside the
selection. The settings window writes: it changes the rows it
is asked to change and rewrites the `.env` in place, leaving every other line
alone. Neither of them delivers anything.

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
| `frame` | What the two small windows share: the palette, the boxes with their labels, and the scroll bar. |
| `vimarea` | A text area with modal editing, used by the overlay. |
| `config` | Settings from the environment and the `.env` in the config directory — read by `Load`, prepared and kept by `Prepare`, and rewritten line by line by `Save`. |
| `draft` | An unfinished prompt on disk, one file per window, written privately and atomically. |
| `win32`, `wintarget`, `winlog` | The same windows on Windows: the chords the daemon claims, the selection read out of the pane, the window each popup opens over, the paste that delivers a prompt into it, and the log a program without a console has to write to. |

`cmd/trans-window` is the composition root for all three windows: it reads the
settings, asks `service` for the one that was chosen, wires the flow to its
targets and starts the program named by `TRANS_PANEL_MODE` — the panel, the
selection, or the settings. `cmd/trans-windowd` is the daemon beside it,
claiming the three chords and handing off to the subcommand each one names.
Nothing below them knows which service is in use or how the popup was opened.

`promptflow` owns the ports it needs — `Translator`, `Target`, `UsageReporter` —
and imports no adapter package, not even `translation`. Where the two
vocabularies have to meet, they meet in the composition root: it is the only
place that knows both a `translation.Usage` and a `promptflow.Usage` exist, and
it adapts one to the other. `promptflow.New` assumes the dependencies it is
handed are real; a composition root is its only caller, and a nil would fail on
the first keystroke.

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

`translation.Options` is a superset — `APIKey`, `TargetLanguage`, `Endpoint` —
and each service takes what applies. That holds while services want the same
three things. The moment one needs something private to it, a project id or a
region, that parsing belongs in the composition root rather than in a struct
every service has to look at: adding fields there makes every provider carry
what one of them needed.

Add a package that implements `Provider`, register it in `Registry()` in
`internal/service/service.go`, and it becomes selectable through
`TRANS_PROVIDER`. The first registered service is the default. A
service that reports what it has spent can implement `UsageReporter`, and the
header shows the count. One that can be told the sentence before the one it is
translating implements `ContextualTranslator`, which is what makes live
translation cheap with it.

## Working on it

```bash
make qa     # formatting, linting, race tests, vulnerability scan
make build
```

Tests are written from the outside in and named as sentences about behaviour.
The overlay, the selection window and the settings window are driven through
`teatest`, which means their tests type keys and read frames rather than
reaching into the model.

# Read mode

The panel exists to write prompts in your own language. Read mode is the other
half of that: text that is already in English — an agent's reply, an error, a
log line — comes back in yours, and nothing is delivered into the window it was
found in.

```bash
trans-window open --read               # read what is on the clipboard
trans-window open --read --capture     # start from the selection in that window
trans-window open --read --target "Windows Terminal"   # positioned like `open`
```

Both invocations are what a hotkey should press. `--read` resolves the window
exactly like `open` does (so the panel lands over the right pane), takes the
draft's place as the source text, and turns the panel around.

## What is different in read mode

- **The draft holds the source.** Whatever you want to read — pasted, typed, or
  captured — sits in the draft box; the second pane shows the result in the read
  language (`TRANS_READ_LANGUAGE`, default `ZH`).
- **`ctrl+d` / `alt+enter` copies the result.** Not "sends", not "fills": the
  translated text goes to the clipboard and a hint says
  `copied to clipboard · esc closes`. The panel stays open and the draft stays
  with it, so one copy may go to several places. `esc` closes.
- **Nothing is kept.** A read-mode draft is text that arrived, not a prompt being
  written: it is never saved for the next session, and there is no confirmation
  step — `TRANS_CONFIRM` does not apply.
- **The same pipeline.** The translator is the one that was configured — same
  provider, same credentials, same usage counting in the header — with only the
  target language overridden to the read language. Live translation, the
  sentence cache, and code protection all behave exactly as when writing.

The header says what changed: a `read` badge, then `service → ZH`. The footer
names the key as `copy`, and offers no `ctrl+r`, because in read mode there is
no delivery to switch between.

## Where the text comes from

### Plain `--read`: the clipboard

If the clipboard has text, the draft opens with it. This is the safe way to
start: nothing is pressed into any window.

### `--capture`: the selection

`--capture` goes and gets the text you had selected in the target window,
before the panel covers it:

1. the clipboard is saved,
2. the target window is brought to the front,
3. the copy chord (`TRANS_CAPTURE_KEYS`, default `ctrl+shift+c`) is pressed into
   it,
4. a short settle wait lets the selection reach the clipboard,
5. the clipboard is read into the draft, and
6. the original clipboard content is restored.

If the capture yields nothing, the draft falls back to whatever was already in
the clipboard. If that is empty too, the draft opens empty and the panel says
why (`nothing captured — …`) as soon as it appears.

**Caveat.** The chord has to be pressed into the window, and with no selection
some terminals treat `ctrl+shift+c` as an interrupt rather than a copy. That is
why plain `--read` exists, and why capture is only reachable through the
explicit `--capture` flag (or a hotkey that spells it out), never as the default
behaviour of `--read`.

## Settings

| Setting | Default | Meaning |
| --- | --- | --- |
| `TRANS_READ_LANGUAGE` | `ZH` | Language read-mode results come back in |
| `TRANS_CAPTURE_KEYS` | `ctrl+shift+c` | Chord `--capture` presses to copy the selection |

`TRANS_LANGUAGE` still names what prompts are translated into; reading has a
language of its own so the two never move each other.

## Keys in read mode

| Key | Action |
| --- | --- |
| `ctrl+d` / `alt+enter` | Copy the result to the clipboard |
| `esc` | Close (from insert mode first, if vim is on) |
| `ctrl+t` | Translate now / retry after error |
| `ctrl+l` | Toggle live translation |
| `tab` | Read the result in full / back to the draft |
| `ctrl+u` | Clear the source text |

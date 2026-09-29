# Sent-prompt history

A prompt that went out once is a prompt that can be found again: what was
written, what the agent received, which window it went into, and when. The
record is written as the prompt is delivered rather than when the panel
closes, so a window hung up on the way out still leaves the prompt behind.

## Opening the record

`ctrl+g` opens it, newest first, one prompt to a line: the time it was sent
and the first line of what was written.

- `↑`/`↓` or `j`/`k` move one prompt at a time, `pgup`/`pgdn` a screenful,
  `home`/`end` (`g`/`G`) jump to the newest and the oldest.
- `enter` puts the selected prompt back into the box exactly as it was
  written, with the translation that went with it beside it. Nothing is spent
  on it — the English was paid for the first time it went out.
- `delete` takes a prompt out of the record: the sentence that was only ever a
  test is not worth keeping. The last entry gone closes the view, because an
  empty record is nothing to look at.
- `esc` closes the record without touching the draft.

While the record is open the keys go to the record and nowhere else: a stray
letter cannot be typed into a box you cannot see.

## How much is kept

The record holds `TRANS_HISTORY_LIMIT` prompts — 500 by default — and drops
the oldest when it grows past that. A line that does not parse is passed
over rather than hiding everything written around it. Trimming a record that
is too long, and taking a prompt out of it, rewrite the file in one piece, so
it is either the old record or the new one and never half of each; the usual
append is a plain append, which two panels writing at once can share.

`TRANS_HISTORY=0` keeps nothing at all, and `ctrl+g` says so instead of
opening an empty view.

## What it holds, and who can read it

The record lives in the state directory — `%LOCALAPPDATA%\trans\state` unless
`TRANS_STATE_DIR` says otherwise — beside the drafts the panel keeps, as
`history.jsonl`. It holds your own prompts: the words you wrote and the
English that went to the agent for them. It is written with the same posture
as the drafts — created for you alone (`0600` where the filesystem keeps
access bits, which Windows does not) — and it never leaves your machine. The
panel only reads it back to offer it to you.

See [README](../README.md#sent-prompt-history) for the two settings.

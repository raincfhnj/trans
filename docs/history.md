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

# Translation memory

While a draft is written, the panel translates it sentence by sentence and
remembers what each sentence cost, so retyping an earlier sentence costs
nothing the second time. That memory used to live only in the running panel;
`tm.jsonl` in the state directory lets it outlive one.

- Each sentence is written as it is learned — no waiting for the panel to
  close, and a window hung up on the way out does not lose what it learned.
- What is written is the sentence as the cache keys it: with the sentence
  before it, and with a code block held out of it as a marker, never the code
  itself. The file holds your own prompts and their English, and it is
  written with the same private-and-atomic posture as the drafts: `0600`
  where the filesystem keeps access bits, rewritten as a whole file moved
  into place so it is never half one version and half another.
- The memory keeps `TRANS_TM_LIMIT` sentences — 5000 by default — in the
  order they were learned, and drops the oldest first after a restart too.
- A line that does not parse, or a file that cannot be read at all, is a cold
  cache: the sentence costs one request to have back, and nothing else is
  lost.
- `TRANS_TM=0` keeps the memory for the session alone, and `TRANS_TM_LIMIT`
  says how much is written.

See [README](../README.md#translation-memory) for the two settings.

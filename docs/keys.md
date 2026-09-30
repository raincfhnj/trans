# Where the API key lives

The key is the one secret this program holds, and it does not have to sit in a
settings file in the clear. On Windows it can be wrapped with the Data
Protection API, which binds the ciphertext to your own logon: another account
on the same machine cannot read it back, and neither can a copy of the file
taken somewhere else.

## The two modes

`TRANS_KEYS` says which one is in force.

| Value | What happens |
| --- | --- |
| `dpapi` (default) | The key is wrapped with DPAPI and kept in `secrets.json` in the configuration directory. The plaintext line is taken out of `.env`. |
| `plain` | Nothing is wrapped and nothing moves: the key stays in `.env`, exactly as it was written. |

The default is the protected one, so an installation that never mentions
`TRANS_KEYS` ends up protected the first time the panel or the daemon starts
after the upgrade.

## What the move does

`UpgradeSecrets` runs at the start of the panel and of the daemon, and is
idempotent — the second run finds nothing to move. The order is deliberate:

1. `TRANS_<PROVIDER>_API_KEY` is preferred over `TRANS_API_KEY`, the same way
   loading the settings reads them, so a key written for one service keeps that
   service's name through the move.
2. The settings file is copied to `.env.bak.<timestamp>` (mode `0600`) before
   anything is rewritten.
3. The key is wrapped and written to `secrets.json`.
4. Only then is the plaintext line removed from `.env` — atomically, with every
   other line left exactly as it stood.

A failure before step 4 leaves the key where it was; a failure after it leaves
the backup. Neither path loses the key.

The key is only ever moved out of the file, never out of the environment: a
`TRANS_API_KEY` handed to one invocation belongs to whoever set it, and the
environment wins over the file at the next load anyway.

## Reading it back

Loading the settings fills the key in from the protected store when the file
carries none, so the services see the same thing either way — the code that
talks to DeepL or an OpenAI-compatible endpoint never knows which mode is in
force.

## Where the files are

```
%APPDATA%\trans\.env            the settings, minus the key once it has moved
%APPDATA%\trans\secrets.json    the wrapped key, one record per setting
%APPDATA%\trans\.env.bak.*      the file as it stood before a move
```

`secrets.json` holds the variable name and the protected bytes, and nothing
else. Deleting the configuration directory takes the key with it.

## On other systems

DPAPI is a Windows thing. Everywhere else the store answers that it is
unsupported, the migration stands down without a note, and `TRANS_KEYS=plain`
is the only mode there is. The tree still builds and its tests still pass.

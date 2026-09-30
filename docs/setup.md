# Setup wizard and doctor

`trans-window setup` is the first-run wizard's read-only half: it walks the
things a working installation needs and says what it found. Every line is a
verdict, the check, and — when something is not right — what to do about it.

```console
$ trans-window setup
PASS  go toolchain: C:\go\bin\go.exe
PASS  settings: C:\Users\you\AppData\Roaming\trans\.env (412 bytes)
PASS  provider key: gtranslate is chosen and needs no key
WARN  programs on PATH: neither trans-window.exe nor trans-windowd.exe is on PATH
     fix: run `make windows` and add bin\ to PATH, or call the programs by full path
PASS  speech synthesis: default voice "Microsoft Zira Desktop" is available via Windows SAPI

3 passed, 1 warnings, 0 failures
```

`trans-window setup doctor` says the same thing — `setup` is the wizard,
`setup doctor` is the name for what it does when you come back to it later.

## Machine-readable output

`--json` prints the same report as one object, so a script or an installer can
read the counts without parsing the drawing:

```console
$ trans-window setup --json
{
  "checks": [
    {
      "name": "go toolchain",
      "verdict": "PASS",
      "detail": "C:\\go\\bin\\go.exe"
    },
    {
      "name": "programs on PATH",
      "verdict": "WARN",
      "detail": "neither trans-window.exe nor trans-windowd.exe is on PATH",
      "fix": "run `make windows` and add bin\\ to PATH, or call the programs by full path"
    }
  ],
  "pass": 1,
  "warn": 1,
  "fail": 0
}
```

`fix` is omitted when there is nothing to do. The exit status is `0` when the
report was printed, whatever the verdicts say: a warning is a finding, not a
broken command. Failures are what you read the lines for.

## The checks

| Check | What it asks | When it is not a PASS |
| --- | --- | --- |
| `go toolchain` | Is `go` on PATH? | **WARN** — only building from source needs it; the fix names where to install it. |
| `settings` | Does `config.Prepare` + `config.Load` read the settings the panel would? | **FAIL** with the reader's own message (a value it refused), **WARN** when there is no config directory. |
| `provider key` | Which service did those settings choose, and does it have what it needs? | **FAIL** when the service could not be built (with the file to configure it in), **WARN** when no service is chosen at all. |
| `programs on PATH` | Are `trans-window.exe` and `trans-windowd.exe` findable? | **WARN** for either missing — the panel opens fine by full path. |
| `speech synthesis` | Can Windows list a default SAPI voice? | **WARN** only. The panel has no audio dependency of its own, so a voice that cannot be listed is never a failure. |

### Credentials never leave the machine

The provider-key line names the variable (`TRANS_API_KEY`, or the scoped
`TRANS_<PROVIDER>_API_KEY`) and at most the last four characters of the value,
so two keys can be told apart without either being printed — in the human
output or the JSON.

### Speech is asked for, best-effort

Nothing under `internal/` plays or synthesises audio. The check asks Windows
through `powershell -NoProfile` whether a default voice can be listed from the
SAPI voice registry — the same question `System.Speech` would be asked, without
taking a dependency on it. PowerShell missing, slow, or returning nothing all
come out as a warning with where to install a voice; the panel itself is
unaffected either way. On other platforms the check reports that it was not
asked.

## The Go toolchain, when it matters

The doctor runs `go version` only to report it. A prebuilt installation never
needs a compiler, so its absence is a warning rather than a failure — the fix
is there for the person who wants to build `trans` from source, and silent for
everyone else.

## How it is put together

`internal/setup` holds the checks behind injected functions — the environment
reader, `config.Load`, `service.Choose`, a PATH lookup, the speech probe — so
the tests drive every branch with fakes and a temp directory instead of a real
machine. `cmd/trans-window/setup.go` wires the real ones in; the Windows-only
pieces (the PATH lookup, the PowerShell voice probe) live behind build tags
with a portable stub, so `go build ./...` and the whole test suite pass on
Linux as well.

```bash
go build ./...     # windows and GOOS=linux
go test ./...      # no network, no real PATH needed
golangci-lint fmt --diff
golangci-lint run
```

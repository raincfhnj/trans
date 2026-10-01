# Releasing

A release is a tag; everything after it is mechanical. The names of the
archives a release must contain are a contract with the package manifests,
written down once in [packaging/README.md](../packaging/README.md) — this
page is the sequence around that contract.

## 1. Test

```bash
make qa      # formatting, vet, lint, race tests, vulnerability scan
go test ./...
```

## 2. Try the archives

```bash
make release VERSION=0.1.0
```

Both zips and `SHA256SUMS.txt` land in `dist/`; look inside one of them —
four files at the archive root, no folder above them:

```bash
unzip -l dist/trans-window-0.1.0-windows-amd64.zip
```

`make release` needs `zip` and `sha256sum` next to `make` (both are there on
Linux, in WSL and in Git Bash; `zip` is one `scoop install zip` away on
Windows). Without a tag of its own, `VERSION` falls back to what
`git describe` says.

## 3. Tag

```bash
git tag v0.1.0
git push origin v0.1.0
```

## 4. What the workflow does

The push of a `v*` tag runs `.github/workflows/release.yml`: it takes the
version from the tag (`v0.1.0` → `0.1.0`), runs `make release` with it, and
publishes a GitHub Release carrying the two zips and `SHA256SUMS.txt`, with
release notes generated from the commits since the last tag. If the archives
or their names are missing, the release step fails rather than publishing an
empty one.

The same workflow can be started by hand from the Actions tab; without a tag
to publish under it only builds and keeps `dist/` as a workflow artifact,
which is how the packaging is rehearsed without cutting a release.

## 5. Scoop and winget

Both point at the assets the workflow just published:

- **scoop** — version, URLs and hashes in the bucket's `bucket/trans.json`
  from `SHA256SUMS.txt`, then a pull request to the bucket; see the
  [scoop instructions](../packaging/README.md#scoop).
- **winget** — one command downloads the archives, fills the manifest hashes
  and opens the pull request against winget-pkgs; see the
  [winget instructions](../packaging/README.md#winget).

```bash
wingetcreate update raincfhnj.trans --version 0.1.0 --urls ... --submit
```

The winget pull request passes validation against the schema and downloads
the release URLs, so it is also a check that the release is intact.

## 6. Look at it as a user would

```bash
scoop install trans
winget install raincfhnj.trans
```

Both install the same two executables the zip holds, and `trans-window open`
is the first thing to run.

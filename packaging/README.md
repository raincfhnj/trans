# Packaging

trans is published as archives on its GitHub releases, and scoop and winget
install from those archives. The manifests in this directory and
`.github/workflows/release.yml` are two sides of one contract: the asset
names are written down once, in the table below, and used verbatim
everywhere else. A name that changes changes here and there in the same
commit.

## The asset names

A release is cut by pushing a tag `v<version>`; `<version>` is the tag
without its leading `v` (`v0.1.0` → `0.1.0`).

| Asset | Name |
| --- | --- |
| Panel and daemon for amd64 | `trans-window-<version>-windows-amd64.zip` |
| Panel and daemon for arm64 | `trans-window-<version>-windows-arm64.zip` |
| Checksums, `sha256sum` format | `SHA256SUMS.txt` |

Both zips hold the same four files at the archive root — `trans-window.exe`,
`trans-windowd.exe`, `README.md`, `LICENSE` — because scoop and winget
unpack the archive as it is: a folder above the executables would break the
`bin` and `NestedInstallerFiles` paths.

An asset is downloaded from

```
https://github.com/raincfhnj/trans/releases/download/v<version>/<name>
```

Note the one asymmetry: the tag directory carries the `v`, the file names do
not. `make release` produces exactly these files in `dist/` on any machine
with `zip` and `sha256sum` next to `make`; the release workflow runs the same
target with `VERSION` set from the tag.

## Scoop

[`scoop/trans.json`](scoop/trans.json) is a scoop bucket manifest. Its
`checkver` and `autoupdate` blocks are what a bucket's automation needs to
bump it by itself: `checkver` reads the GitHub releases, `autoupdate`
rewrites both URLs with `$version` and takes the hash from the release's
`SHA256SUMS.txt`.

To publish:

1. Fork or create a bucket — a scoop bucket is an ordinary repository whose
   manifests live in `bucket/`.
2. Copy `scoop/trans.json` there as `bucket/trans.json`.
3. For a release, set three things, all visible at once:
   - `version` — `<version>`, without the `v`;
   - both `architecture.*.url`s — the asset URLs from the table above;
   - both `architecture.*.hash`es — the zip's SHA-256 from `SHA256SUMS.txt`
     (first column of the matching line).

   The very first release replaces the `0.0.0` placeholders.
4. Open the pull request — or push straight to a bucket of your own.
   `scoop install trans` picks it up from there.

## Winget

[`winget/`](winget/) holds the three files of a winget-pkgs manifest, with
`{{...}}` placeholders. In a submission they belong at

```
manifests/r/raincfhnj/trans/<version>/
```

in a fork of [microsoft/winget-pkgs](https://github.com/microsoft/winget-pkgs),
with `{{VERSION}}` replaced by `<version>` (again without the `v`) and
`{{SHA256_AMD64}}` / `{{SHA256_ARM64}}` by the two zip hashes from
`SHA256SUMS.txt`.

For every release after the first, `wingetcreate` does that for you: it
downloads the archives, writes their hashes into the manifest and opens the
pull request.

```bash
wingetcreate update raincfhnj.trans \
  --version 0.1.0 \
  --urls 'https://github.com/raincfhnj/trans/releases/download/v0.1.0/trans-window-0.1.0-windows-amd64.zip|x64' \
         'https://github.com/raincfhnj/trans/releases/download/v0.1.0/trans-window-0.1.0-windows-arm64.zip|arm64' \
  --submit
```

The `|x64` and `|arm64` suffixes tell it which installer entry each URL
belongs to. The first submission cannot update a package that does not exist
yet: fill the three templates, put them in the path above on a branch of your
winget-pkgs fork, and open the pull request by hand. A GitHub token with
`repo` scope is needed for `--submit`; `wingetcreate token -s` caches one.

Every winget-pkgs pull request is validated against the schema and against
the installer URLs, so a mistake in this naming contract fails there — before
anyone can install it.

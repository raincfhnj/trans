#!/usr/bin/env bash
# Runs on `herdr plugin install` (or `herdr plugin link`). Builds the overlay
# binary into the plugin root, so the first keypress finds it there.
set -euo pipefail

cd "$(dirname "$0")/.."
mkdir -p bin

command -v go >/dev/null || {
	echo "trans: no Go toolchain to build the plugin with" >&2
	exit 1
}

# Without the symbol table and DWARF the binary is a third smaller, which is
# a third less to read from disk the first time a keypress runs it.
go build -trimpath -ldflags "-s -w" -o bin/trans ./cmd/trans

# Run it once so the first real keypress does not wait for the operating system
# to page in and check a binary it has never seen.
./bin/trans warm >/dev/null 2>&1 || true

GOBIN  := $(shell go env GOPATH)/bin

# The Windows build is written down once here and used by every target that
# makes one: the binaries a release ships and the binaries a developer runs have
# to be the same program, down to the console the daemon does not have and the
# symbols it does not carry. `-H windowsgui` is what makes trans-windowd a
# program without a console window; without it, the daemon a release would ship
# opens one.
WIN_EXE := -trimpath -ldflags "-s -w"
WIN_GUI := -trimpath -ldflags "-s -w -H windowsgui"

.PHONY: all build windows test race cover fmt fmt-check lint vet vuln qa clean tools

all: qa build

# `build` is the windows target: what it writes to bin/ is what users run, so it
# is built the way the release builds it rather than with the host's defaults.
build: windows

# The environment a Windows build wants, as target-specific exports rather than
# as `CGO_ENABLED=0 GOOS=windows go build`: the inline form is a POSIX shell's
# and cmd.exe does not have it, so `make windows` would fail on Windows itself.
windows: export CGO_ENABLED = 0
windows: export GOOS = windows
windows: export GOARCH = amd64

# The panel for a native terminal on Windows: the window it opens over, and the
# hotkey that opens it. The second has no console, so it is built as one.
windows:
	go build $(WIN_EXE) -o bin/trans-window.exe  ./cmd/trans-window
	go build $(WIN_GUI) -o bin/trans-windowd.exe ./cmd/trans-windowd

test:
	go test ./...

race:
	go test -race ./...

# The whole per-function report, whose last line is the total. `go tool cover`
# prints that line itself, so this target needs no tail, awk or shell
# substitution next to make and runs the same on Windows, Linux and macOS.
cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

fmt:
	$(GOBIN)/golangci-lint fmt

fmt-check:
	$(GOBIN)/golangci-lint fmt --diff

lint:
	$(GOBIN)/golangci-lint run

# `go vet` is what a reader runs by hand, so it is a gate of its own here: a
# standard check that fails while `make qa` says everything is fine is a gate
# nobody can trust. Its one exception is internal/win32, where a Windows call
# hands an address back as a uintptr and the conversion that makes it usable is
# exactly what vet's unsafeptr check cannot be told is safe. That check is
# turned off for that package alone — the sites carry the invariant in a
# comment — and the exception lives in tools/vet, in Go, so this target needs
# no grep or shell substitution and runs the same on every system and in CI.
vet:
	go run ./tools/vet

vuln:
	$(GOBIN)/govulncheck ./...

qa: fmt-check vet lint race vuln

# The exact linter and scanner CI runs, so `make qa` and the workflow are the
# same gate rather than two that drift: a version bump happens here and in
# .github/workflows/ci.yml in one change.
tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2
	go install golang.org/x/vuln/cmd/govulncheck@v1.8.0

clean:
	-rm -rf bin dist coverage coverage.out

# The archives a release ships: both exes next to README and LICENSE in one
# zip per architecture, plus the checksums the scoop and winget manifests
# read. The build flags are the windows target's, per architecture. VERSION
# is the tag without its leading v (v0.1.0 becomes 0.1.0), and the file names
# are the contract the manifests under packaging/ are written against — they
# change together or not at all (packaging/README.md). Needs zip and
# sha256sum next to make, so it runs in a POSIX shell (Linux, macOS, WSL, Git
# Bash); .github/workflows/release.yml runs this target on ubuntu. The version
# is read lazily — only a target that asks for $(VERSION) shells out to git, so
# an unpacked source tree can still run `make test` — and stripped with make's
# own patsubst rather than sed, so reading it needs no shell either.
TAG     = $(shell git describe --tags --always 2>/dev/null)
VERSION ?= $(patsubst v%,%,$(TAG))

.PHONY: release
release:
	rm -rf dist
	mkdir -p dist/stage
	cp README.md LICENSE dist/stage/
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(WIN_EXE) -o dist/stage/trans-window.exe  ./cmd/trans-window
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(WIN_GUI) -o dist/stage/trans-windowd.exe ./cmd/trans-windowd
	cd dist/stage && zip -q -r ../trans-window-$(VERSION)-windows-amd64.zip trans-window.exe trans-windowd.exe README.md LICENSE
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build $(WIN_EXE) -o dist/stage/trans-window.exe  ./cmd/trans-window
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build $(WIN_GUI) -o dist/stage/trans-windowd.exe ./cmd/trans-windowd
	cd dist/stage && zip -q -r ../trans-window-$(VERSION)-windows-arm64.zip trans-window.exe trans-windowd.exe README.md LICENSE
	cd dist && sha256sum trans-window-$(VERSION)-windows-*.zip > SHA256SUMS.txt
	rm -rf dist/stage

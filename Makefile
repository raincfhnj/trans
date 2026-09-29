GOBIN  := $(shell go env GOPATH)/bin

.PHONY: all build windows test race cover fmt fmt-check lint vuln qa clean tools

all: qa build

build:
	go build -o bin/trans-window.exe ./cmd/trans-window
	go build -o bin/trans-windowd.exe ./cmd/trans-windowd

# The panel for a native terminal on Windows: the window it opens over, and the
# hotkey that opens it. The second has no console, so it is built as one.
windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w"               -o bin/trans-window.exe  ./cmd/trans-window
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -H windowsgui" -o bin/trans-windowd.exe ./cmd/trans-windowd

test:
	go test ./...

race:
	go test -race ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

fmt:
	$(GOBIN)/golangci-lint fmt

fmt-check:
	$(GOBIN)/golangci-lint fmt --diff

lint:
	$(GOBIN)/golangci-lint run

vuln:
	$(GOBIN)/govulncheck ./...

qa: fmt-check lint race vuln

tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest

clean:
	rm -rf bin coverage.out

# The archives a release ships: both exes next to README and LICENSE in one
# zip per architecture, plus the checksums the scoop and winget manifests
# read. The build flags are the windows target's, per architecture. VERSION
# is the tag without its leading v (v0.1.0 becomes 0.1.0), and the file names
# are the contract the manifests under packaging/ are written against — they
# change together or not at all (packaging/README.md). Needs zip and
# sha256sum next to make; .github/workflows/release.yml runs this target.
VERSION ?= $(shell git describe --tags --always | sed 's/^v//')

.PHONY: release
release:
	rm -rf dist
	mkdir -p dist/stage
	cp README.md LICENSE dist/stage/
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w"               -o dist/stage/trans-window.exe  ./cmd/trans-window
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -H windowsgui" -o dist/stage/trans-windowd.exe ./cmd/trans-windowd
	cd dist/stage && zip -q -r ../trans-window-$(VERSION)-windows-amd64.zip trans-window.exe trans-windowd.exe README.md LICENSE
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -trimpath -ldflags "-s -w"               -o dist/stage/trans-window.exe  ./cmd/trans-window
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -trimpath -ldflags "-s -w -H windowsgui" -o dist/stage/trans-windowd.exe ./cmd/trans-windowd
	cd dist/stage && zip -q -r ../trans-window-$(VERSION)-windows-arm64.zip trans-window.exe trans-windowd.exe README.md LICENSE
	cd dist && sha256sum trans-window-$(VERSION)-windows-*.zip > SHA256SUMS.txt
	rm -rf dist/stage

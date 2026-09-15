BINARY := bin/trans
GOBIN  := $(shell go env GOPATH)/bin

.PHONY: all build cross windows test race cover fmt fmt-check lint vuln qa clean tools

all: qa build

build:
	go build -o $(BINARY) ./cmd/trans

# The panel for a native terminal on Windows: the window it opens over, and the
# hotkey that opens it. The second has no console, so it is built as one.
windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w"                 -o bin/trans-window.exe  ./cmd/trans-window
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -H windowsgui"   -o bin/trans-windowd.exe ./cmd/trans-windowd

# herdr runs on macOS and Linux; a release carries all four binaries.
cross:
	CGO_ENABLED=0 GOOS=linux  GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o bin/trans_linux_amd64  ./cmd/trans
	CGO_ENABLED=0 GOOS=linux  GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o bin/trans_linux_arm64  ./cmd/trans
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o bin/trans_darwin_amd64 ./cmd/trans
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o bin/trans_darwin_arm64 ./cmd/trans

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

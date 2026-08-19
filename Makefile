BINARY  := cyber-tui
OUT     := dist/$(BINARY)

VERSION := $(shell git describe --tags --always --dirty)
COMMIT  := $(shell git rev-parse --short HEAD)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -ldflags "\
  -X github.com/ragnar/cyber-tui/internal/version.Version=$(VERSION) \
  -X github.com/ragnar/cyber-tui/internal/version.Commit=$(COMMIT) \
  -X github.com/ragnar/cyber-tui/internal/version.Date=$(DATE)"

.PHONY: build
build:
	go build $(LDFLAGS) -o $(OUT) ./cmd/cyber-tui

.PHONY: build-all
build-all:
	GOOS=linux  GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY)-linux-amd64   ./cmd/cyber-tui
	GOOS=linux  GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY)-linux-arm64   ./cmd/cyber-tui
	GOOS=linux  GOARCH=arm GOARM=7 go build $(LDFLAGS) -o dist/$(BINARY)-linux-armv7l ./cmd/cyber-tui
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY)-darwin-amd64  ./cmd/cyber-tui
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY)-darwin-arm64  ./cmd/cyber-tui
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY)-windows-amd64.exe ./cmd/cyber-tui

.PHONY: lint
lint:
	go vet ./...
	staticcheck ./...

.PHONY: vuln
vuln:
	govulncheck ./...

.PHONY: fetch
fetch:
	go run ./cmd/apifetch $(ARGS)

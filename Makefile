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

.PHONY: fetch
fetch:
	go run ./cmd/apifetch $(ARGS)

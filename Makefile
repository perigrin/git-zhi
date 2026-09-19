.PHONY: all build install test vet clean cross-compile

all: build

# VERSION, COMMIT and LDFLAGS are defined here and nowhere else. Every build —
# local, cross-compiled, and the release workflow — goes through this file, so
# a binary's reported version does not depend on how it was produced.
#
# The leading v is stripped from the tag: `git describe` emits v0.5.2 while
# published releases have always reported 0.5.2, and the published format is
# the one people have written checks against.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
OUT     ?= git-zhi
LDFLAGS  = -s -w \
	-X 'github.com/perigrin/git-zhi/internal/version.Version=$(VERSION)' \
	-X 'github.com/perigrin/git-zhi/internal/version.BuildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)' \
	-X 'github.com/perigrin/git-zhi/internal/version.CommitHash=$(COMMIT)'

build:
	go build -ldflags="$(LDFLAGS)" -o $(OUT) ./cmd/git-zhi/

PREFIX ?= $(HOME)/.local
install: build
	install -d $(PREFIX)/bin
	install -m 755 $(OUT) $(PREFIX)/bin/git-zhi
	$(PREFIX)/bin/git-zhi version

test:
	go test ./... -count=1

vet:
	go vet ./...

clean:
	rm -f git-zhi
	rm -f git-zhi-linux-amd64 git-zhi-linux-arm64 git-zhi-darwin-arm64 git-zhi-windows-amd64.exe
	go clean ./...

cross-compile:
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 $(MAKE) build OUT=git-zhi-linux-amd64
	CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 $(MAKE) build OUT=git-zhi-linux-arm64
	CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 $(MAKE) build OUT=git-zhi-darwin-arm64
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(MAKE) build OUT=git-zhi-windows-amd64.exe

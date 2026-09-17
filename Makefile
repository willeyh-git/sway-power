# sway-power build/test entry points.
# The README's "Build" section documents the plain `go build`; this file is
# the single place that pins the version ldflags so `make build` and
# `make release` embed the same version string.

GO       ?= go
BINARY   ?= sway-power

# git describe gives v1.2.3, v1.2.3-4-gabc1234, or just abc1234 when no
# tag exists yet; --always falls back to a short hash.
VERSION  ?= $(shell git describe --tags --always --dirty)
LDFLAGS  := -X main.version=$(VERSION)

.PHONY: build test vet release

# Development build (what the README instructs, plus the version).
build:
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/sway-power

# The standard validation gate. -race is mandatory: the daemon has
# multiple concurrent lifecycles (inhibitor, monitor, preferences
# watcher) and only the race detector covers them.
test:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

# Release build: stripped, no DWARF, version from the tag.
# The Fyne GUI is cgo; this targets the host only (linux/amd64 today).
release:
	$(GO) build -trimpath -ldflags '$(LDFLAGS) -s -w' -o $(BINARY) ./cmd/sway-power

# Oikos agent — build, test, lint and benchmark entrypoints.
#
# CGO stays off, always (CLAUDE.md §4.5): pure-Go D-Bus, a static binary, no
# coupling to the host's glibc/libsystemd. Every recipe below inherits that.

BINARY := oikos-agent
PKG    := ./cmd/oikos-agent
DIST   := dist

# Build metadata, injected into main via -ldflags -X. Overridable by the
# release pipeline; the defaults describe a local working-tree build.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.date=$(DATE)

export CGO_ENABLED := 0

.DEFAULT_GOAL := build

.PHONY: build test bench lint vet staticcheck tidy clean help

## build: compile the agent binary for the host platform into $(DIST)/.
build:
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/$(BINARY) $(PKG)

## test: run the fixture-based test suite (no root/systemd/docker required).
test:
	go test ./...

## bench: run sampler hot-path benchmarks (kept as a CI regression gate).
bench:
	go test -run '^$$' -bench . -benchmem ./...

## vet: run go vet.
vet:
	go vet ./...

## staticcheck: run staticcheck (installed on demand if absent).
staticcheck:
	@command -v staticcheck >/dev/null 2>&1 || { \
		echo "staticcheck not found; install with:"; \
		echo "  go install honnef.co/go/tools/cmd/staticcheck@latest"; \
		exit 1; \
	}
	staticcheck ./...

## lint: go vet + staticcheck.
lint: vet staticcheck

## tidy: sync go.mod/go.sum.
tidy:
	go mod tidy

## clean: remove build output.
clean:
	rm -rf $(DIST)

## help: list available targets.
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //'

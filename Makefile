# NetSwitcher build matrix. The full GUI build needs MinGW-w64 (gcc) for the
# cgo webview linkage; `make build-cli` produces a service/CLI-only binary
# with no C toolchain requirement.

VERSION ?= 0.1.0
# -H windowsgui: GUI subsystem (no console window on double-click). CLI
# subcommands re-attach to the parent console via winutil.AttachParentConsole.
LDFLAGS := -X main.version=$(VERSION) -H windowsgui
BINARY  := build/bin/netswitcher.exe

.PHONY: all build frontend build-cli test clean fmt help

all: build

## build: build the front-end, then compile the single binary (needs gcc).
##   Wails requires the `desktop` build tag — without it the embedded runtime
##   shows "Wails applications will not build without the correct build tags".
build: frontend
	@mkdir -p build/bin
	CGO_ENABLED=1 go build -tags "desktop,production" -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/netswitcher

## build-cli: compile without CGO (no GUI; service/CLI only, no gcc needed).
build-cli:
	@mkdir -p build/bin
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/netswitcher

## frontend: install npm deps and build the Svelte front-end into dist/.
frontend:
	cd frontend && npm install && npm run build

## icon: compile build/windows/icon.ico into resource.syso (exe icon).
##        Use this when you've replaced icon.ico with your own .ico.
icon:
	rsrc -ico build/windows/icon.ico -arch amd64 -o cmd/netswitcher/resource.syso

## icon-gen: regenerate build/windows/icon.ico from the Go generator (the
##           default dark-navy double-arrow design). Overwrites icon.ico.
icon-gen:
	go run build/windows/generate-icon.go
	rsrc -ico build/windows/icon.ico -arch amd64 -o cmd/netswitcher/resource.syso

## dev: run the Wails dev loop (hot reload).
dev:
	wails dev

## test: run all unit tests.
test:
	go test ./...

## fmt: gofmt + go vet tidy.
fmt:
	gofmt -w .
	go vet ./...
	go mod tidy

clean:
	rm -rf build/bin
	rm -rf frontend/dist

help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //'

# NetSwitcher build matrix. The full GUI build needs MinGW-w64 (gcc) for the
# cgo webview linkage; `make build-cli` produces a service/CLI-only binary
# with no C toolchain requirement.

VERSION ?= 0.1.0
LDFLAGS := -X main.version=$(VERSION)
BINARY  := netswitcher.exe

.PHONY: all build frontend build-cli test clean fmt help

all: build

## build: build the front-end, then compile the single binary (needs gcc).
build: frontend
	CGO_ENABLED=1 go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/netswitcher

## build-cli: compile without CGO (no GUI; service/CLI only, no gcc needed).
build-cli:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/netswitcher

## frontend: install npm deps and build the Svelte front-end into dist/.
frontend:
	cd frontend && npm install && npm run build

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
	rm -f $(BINARY)
	rm -rf frontend/dist

help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //'

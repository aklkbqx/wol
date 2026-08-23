GO ?= go
PREFIX ?= $(shell $(GO) env GOPATH)

.PHONY: test check build install run clean

test:
	$(GO) test ./...

check:
	$(GO) vet ./...

build:
	mkdir -p dist
	$(GO) build -trimpath -ldflags "-s -w" -o dist/wol ./cmd/wol

install:
	$(GO) install -trimpath ./cmd/wol

run:
	$(GO) run ./cmd/wol

clean:
	$(GO) clean

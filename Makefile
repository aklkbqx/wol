GO ?= go
WEB_DIR := web
WEB_BUILD := $(WEB_DIR)/build

.PHONY: test check web-install web-check web-build build run

test:
	$(GO) test ./...

check:
	$(GO) vet ./...
	$(MAKE) web-check

web-install:
	cd $(WEB_DIR) && npm install

web-check:
	cd $(WEB_DIR) && npm run check

web-build:
	cd $(WEB_DIR) && npm run build

build: web-build
	mkdir -p dist
	$(GO) build -trimpath -ldflags "-s -w" -o dist/wol ./cmd/wol

run:
	$(GO) run ./cmd/wol server --db wol.db --web-dir $(WEB_BUILD)

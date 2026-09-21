GO ?= go
PREFIX ?= $(shell $(GO) env GOPATH)
BINDIR ?= $(PREFIX)/bin
PLIST := $(CURDIR)/packaging/macos/Info.plist
LDFLAGS := -s -w
CGO_FOR_BUILD ?= 0
CODESIGN_CMD := true

BUILD_GOOS := $(or $(GOOS),$(shell $(GO) env GOOS))
BUILD_HOST := $(shell uname -s)

ifeq ($(BUILD_HOST),Darwin)
ifeq ($(BUILD_GOOS),darwin)
CGO_FOR_BUILD := 1
LDFLAGS += -linkmode=external -extldflags=-Wl,-sectcreate,__TEXT,__info_plist,$(PLIST)
CODESIGN_CMD := codesign --force --sign - --identifier com.aklkbqx.wol
endif
endif

.PHONY: test check build install run clean

test:
	$(GO) test ./...

check:
	$(GO) vet ./...

build:
	mkdir -p dist
	CGO_ENABLED=$(CGO_FOR_BUILD) $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/wol ./cmd/wol
	$(CODESIGN_CMD) dist/wol

install: build
	mkdir -p "$(DESTDIR)$(BINDIR)"
	install -m 755 dist/wol "$(DESTDIR)$(BINDIR)/wol"

run:
	$(GO) run ./cmd/wol

clean:
	$(GO) clean
	rm -rf dist

.PHONY: verify package
verify:
	sh scripts/verify.sh

package:
	sh scripts/package.sh

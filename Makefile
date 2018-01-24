PKG    = github.com/sapcc/swift-http-import
PREFIX := /usr

all: build/swift-http-import

# force people to use golangvend
GO            := GOPATH=$(CURDIR)/.gopath GOBIN=$(CURDIR)/build go
GO_BUILDFLAGS :=
GO_LDFLAGS    := -s -w

build/swift-http-import: pkg/util/version.go FORCE
	$(GO) install $(GO_BUILDFLAGS) -ldflags '$(GO_LDFLAGS)' '$(PKG)'
pkg/util/version.go: FORCE
	util/find_version.sh

check: all
	bash tests.sh http
	bash tests.sh swift
	echo -e '\e[1;32mSuccess!\e[0m'

install: FORCE all
	install -D -m 0755 build/swift-http-import "$(DESTDIR)$(PREFIX)/bin/swift-http-import"

vendor: FORCE
	# vendoring by https://github.com/holocm/golangvend
	@golangvend

.PHONY: FORCE

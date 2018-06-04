PKG    = github.com/sapcc/swift-http-import
PREFIX := /usr

all: build/swift-http-import

# force people to use golangvend
GO            := GOPATH=$(CURDIR)/.gopath GOBIN=$(CURDIR)/build go
GO_BUILDFLAGS :=
GO_LDFLAGS    := -s -w

build/swift-http-import: FORCE
	$(GO) install $(GO_BUILDFLAGS) -ldflags '$(GO_LDFLAGS) -X github.com/sapcc/swift-http-import/pkg/util.Version=$(shell util/find_version.sh)' '$(PKG)'

check: all
	bash tests.sh http
	bash tests.sh swift
	@printf '\e[1;32mSuccess!\e[0m\n'

install: FORCE all
	install -D -m 0755 build/swift-http-import "$(DESTDIR)$(PREFIX)/bin/swift-http-import"

vendor: FORCE
	# vendoring by https://github.com/holocm/golangvend
	@golangvend

.PHONY: FORCE

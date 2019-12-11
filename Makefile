PKG     = github.com/sapcc/swift-http-import
PREFIX := /usr

all: build/swift-http-import

# NOTE: This repo uses Go modules, and uses a synthetic GOPATH at
# $(CURDIR)/.gopath that is only used for the build cache. $GOPATH/src/ is
# empty.
GO            := GOPATH=$(CURDIR)/.gopath GOBIN=$(CURDIR)/build go
GO_BUILDFLAGS :=
GO_LDFLAGS    := -s -w

build/swift-http-import: FORCE
	$(GO) install $(GO_BUILDFLAGS) -ldflags '$(GO_LDFLAGS) -X github.com/sapcc/swift-http-import/pkg/util.Version=$(shell util/find_version.sh)' '$(PKG)'

# which packages to test with static checkers?
GO_ALLPKGS := $(PKG) $(shell $(GO) list $(GO_BUILDFLAGS) $(PKG)/pkg/...)
# which packages to test with `go test`?
GO_TESTPKGS := $(shell $(GO) list $(GO_BUILDFLAGS) -f '{{if .TestGoFiles}}{{.ImportPath}}{{end}}' $(PKG) $(PKG)/pkg/...)

# detailed test run
check: all quick-check FORCE
	bash tests.sh http
	bash tests.sh swift
	@printf "\e[1;32m>> All tests successful.\e[0m\n"

# quick unit test run
quick-check: all static-check $(addprefix quick-check-,$(subst /,_,$(GO_TESTPKGS))) FORCE
quick-check-%:
	@printf "\e[1;36m>> go test $(subst _,/,$*)\e[0m\n"
	$(GO) test $(GO_BUILDFLAGS) -ldflags '$(GO_LDFLAGS)' $(subst _,/,$*)

static-check: FORCE
	@if ! hash golint 2>/dev/null; then printf "\e[1;36m>> Installing golint...\e[0m\n"; go get -u golang.org/x/lint/golint; fi
	@printf "\e[1;36m>> gofmt\e[0m\n"
	@if s="$$(gofmt -s -l *.go cmd pkg 2>/dev/null)" && test -n "$$s"; then printf ' => %s\n%s\n' gofmt  "$$s"; false; fi
	@printf "\e[1;36m>> golint\e[0m\n"
	@if s="$$(golint . && find cmd pkg -type d -exec golint {} \; 2>/dev/null)" && test -n "$$s"; then printf ' => %s\n%s\n' golint "$$s"; false; fi
	@printf "\e[1;36m>> go vet\e[0m\n"
	@$(GO) vet $(GO_ALLPKGS)

install: FORCE all
	install -D -m 0755 build/swift-http-import "$(DESTDIR)$(PREFIX)/bin/swift-http-import"

clean: FORCE
	rm -rf -- build

vendor: FORCE
	@$(GO) mod tidy
	@$(GO) mod vendor

.PHONY: FORCE

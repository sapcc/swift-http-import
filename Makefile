all: swift-http-import

# force people to use golangvend
GOCC := env GOPATH=$(CURDIR)/.gopath go
GOFLAGS := -ldflags '-s -w'

swift-http-import: *.go
	$(GOCC) build $(GOFLAGS) -o $@ github.com/sapcc/swift-http-import

check: all
	bash tests.sh http
	bash tests.sh swift
	echo -e '\e[1;32mSuccess!\e[0m'

vendor:
	@golangvend
.PHONY: vendor

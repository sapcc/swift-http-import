all: swift-http-import

# force people to use golangvend
GOCC := env GOPATH=$(CURDIR)/.gopath go
GOFLAGS := -ldflags '-s -w'

swift-http-import: *.go
	$(GOCC) build $(GOFLAGS) -o $@ github.com/sapcc/swift-http-import

vendor:
	@golangvend
.PHONY: vendor

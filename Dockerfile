FROM golang:1.17-alpine3.13 as builder
RUN apk add --no-cache make

COPY . /src
RUN make -C /src install PREFIX=/pkg GO_BUILDFLAGS='-mod vendor'

################################################################################

FROM alpine:3.13
LABEL source_repository="https://github.com/sapcc/swift-http-import"

RUN apk add --no-cache tini ca-certificates
COPY --from=builder /pkg/ /usr/
ENTRYPOINT ["/sbin/tini", "--", "/usr/bin/swift-http-import"]

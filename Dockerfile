FROM golang:1.12-alpine as builder
WORKDIR /x/src/github.com/sapcc/swift-http-import/
RUN apk add --no-cache make

COPY . .
RUN make install PREFIX=/pkg

################################################################################

FROM alpine:latest
MAINTAINER "Stefan Majewsky <stefan.majewsky@sap.com>"
RUN apk add --no-cache tini ca-certificates

COPY --from=builder /pkg/ /usr/
ENTRYPOINT ["/sbin/tini", "--", "/usr/bin/swift-http-import"]

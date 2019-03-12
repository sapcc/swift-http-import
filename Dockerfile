FROM golang:1.12-alpine as builder
WORKDIR /x/src/github.com/sapcc/swift-http-import/
RUN apk add --no-cache curl make openssl bash && \
    mkdir -p /pkg/bin/ && \
    curl -L https://github.com/Yelp/dumb-init/releases/download/v1.2.0/dumb-init_1.2.0_amd64 > /pkg/bin/dumb-init && \
    chmod +x /pkg/bin/dumb-init

COPY . .
RUN make install PREFIX=/pkg

################################################################################

FROM alpine:latest
MAINTAINER "Stefan Majewsky <stefan.majewsky@sap.com>"
RUN apk add --no-cache ca-certificates

COPY --from=builder /pkg/ /usr/
ENTRYPOINT ["/usr/bin/dumb-init", "--", "/usr/bin/swift-http-import"]

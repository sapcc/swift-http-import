FROM alpine:latest
MAINTAINER "Stefan Majewsky <stefan.majewsky@sap.com>"

# wget(1) is only used to retrieve dumb-init
RUN apk update && \
    apk add wget && \
    wget -O /bin/dumb-init https://github.com/Yelp/dumb-init/releases/download/v1.2.0/dumb-init_1.2.0_amd64 && \
    chmod +x /bin/dumb-init && \
    apk del wget

ADD swift-http-import /bin/swift-http-import
ENTRYPOINT ["/bin/dumb-init", "--", "/bin/swift-http-import"]

# swift-http-import

This tool imports files from an HTTP server into a Swift container. Given an input URL, it recurses through the directory
listings on that URL, and mirrors all files that it finds into Swift. It will take advantage of `Last-Modified` and
`Etag` response headers to avoid repeated downloads of the same content, so best performance is ensured the HTTP server
handles `If-Modified-Since` and `If-None-Match` request headers correctly.

## Implicit assumptions

The HTTP server must present the contents of directories using standard directory listings, as can be generated
automatically by many HTTP servers. In detail, that means that `GET` on a directory should return an HTML page that

- links to all files in this directory with an `<a>` tag whose `href` is just the filename (i.e. relative, not absolute URL)
- links to all directories below this directory with an `<a>` tag whose `href` is just the directory name plus
  a trailing slash (i.e. relative, not absolute URL)

Absolute URLs containing a protocol and domain are ignored, as are relative URLs containing `..` path elements.

## Installation

To build the binary:

```bash
make
```

The binary can also be installed with `go get`:
```bash
go get github.com/sapcc/swift-http-import
```

To build the Docker container:

```bash
make GOFLAGS="-ldflags '-w -linkmode external -extldflags -static'" && docker build .
```

## Usage

Call with the path to a configuration file, that should look like this:

```yaml
swift:
  auth_url: https://my.keystone.local:5000/v3
  user_name: uploader
  user_domain_name: Default
  project_name: datastore
  project_domain_name: Default
  password: 20g82rzg235oughq

jobs:
  - from: http://de.archive.ubuntu.com/ubuntu/
    to:   mirror/ubuntu-repos
```

The first paragraph contains the authentication parameters for OpenStack's Identity v3 API. Optional a `region_name`
can be specified.
Each sync job contains the source URL as `from`, and `to` has the target container name, optionally followed by an 
object name prefix in the target container. For example, in the case above, the file

```
http://de.archive.ubuntu.com/ubuntu/pool/main/p/pam/pam_1.1.8.orig.tar.gz
```

would be synced to the `mirror` container as

```
ubuntu-repos/pool/main/p/pam/pam_1.1.8.orig.tar.gz
```

There is also support for SSL client based authentication against the source. Hereby the server CA is optional.
```yaml
jobs:
  - from: http://de.archive.ubuntu.com/ubuntu/
    to:   mirror/ubuntu-repos
    cert: /path/to/client.pem
    key:  /path/to/client-key.pem
    ca:   /path/to/server-ca.pem
```


## Log output

Log output on `stderr` is very sparse by default. Errors are always reported, and a final count will appear at the end like this:

```
2016/12/19 14:28:23 INFO: 103 dirs scanned, 1496 files found, 167/170 files transferred
```

In this case, all sources contain 103 directories and 1496 files. 170 files were found to be newer on the source and
thus need transfer. 167 of them were successfully transferred, which means that 3 file transfers failed (so there should
be 3 errors in the log).

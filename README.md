# swift-http-import

* [Do NOT use if...](#do-not-use-if)
* [Implicit assumptions](#implicit-assumptions)
* [Installation](#installation)
* [Usage](#usage)
  * [Source specification](#source-specification)
  * [File selection](#file-selection)
  * [Transfer behavior: Segmenting](#transfer-behavior-segmenting)
  * [Transfer behavior: Expiring objects](#transfer-behavior-expiring-objects)
  * [Performance](#performance)
* [Log output](#log-output)
* [StatsD metrics](#statsd-metrics)

This tool imports files from an HTTP server into a Swift container. Given an input URL, it recurses through the directory
listings on that URL, and mirrors all files that it finds into Swift. It will take advantage of `Last-Modified` and
`Etag` response headers to avoid repeated downloads of the same content, so best performance is ensured the HTTP server
handles `If-Modified-Since` and `If-None-Match` request headers correctly.

## Do NOT use if...

* ...you have access to the source filesystem. Just use the normal [`swift upload`](https://docs.openstack.org/python-swiftclient/latest/) instead, it's much more efficient.
* ...you need to import a lot of small files exactly once. Download them all and pack them into a tarball, and send them to Swift in one step with a [bulk upload](https://www.swiftstack.com/docs/admin/middleware/bulk.html#uploading-archives).

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

Or just grab a pre-compiled binary from the [release list](https://github.com/sapcc/swift-http-import/releases).

To build the Docker container:

```bash
docker build .
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
  - from:
      url: http://de.archive.ubuntu.com/ubuntu/
    to:
      container: mirror
      object_prefix: ubuntu-repos
```

The first paragraph contains the authentication parameters for OpenStack's Identity v3 API. Optionally a `region_name`
can be specified, but this is only required if there are multiple regions to choose from.

Each sync job contains the source URL as `from.url`, and `to.container` has the target container name, optionally paired with an
object name prefix in the target container. For example, in the case above, the file

```
http://de.archive.ubuntu.com/ubuntu/pool/main/p/pam/pam_1.1.8.orig.tar.gz
```

would be synced to the `mirror` container as

```
ubuntu-repos/pool/main/p/pam/pam_1.1.8.orig.tar.gz
```

The order of jobs is significant: Source trees will be scraped in the order indicated by the `jobs` list.

### Source specification

The source in `jobs[].from` can also be a private Swift container if Swift credentials are specified instead of a source URL.

```yaml
jobs:
  - from:
      auth_url:            https://my.keystone.local:5000/v3
      user_name:           uploader
      user_domain_name:    Default
      project_name:        datastore
      project_domain_name: Default
      password:            20g82rzg235oughq
      container:           upstream-mirror
      object_prefix:       repos/ubuntu
    to:
      container: mirror
      object_prefix: ubuntu-repos
```

If a source URL is used, you can also pin the server's CA certificate, and specify a TLS client certificate (including
private key) that will be used by the HTTP client.

```yaml
jobs:
  - from:
      url:  http://de.archive.ubuntu.com/ubuntu/
      cert: /path/to/client.pem
      key:  /path/to/client-key.pem
      ca:   /path/to/server-ca.pem
    to:
      container: mirror
      object_prefix: ubuntu-repos
```

### File selection

For each job, you may supply three [regular expressions](https://golang.org/pkg/regexp/syntax/) to influence which files
are transferred:

* `except`: Files and subdirectories whose path matches this regex will not be transferred.
* `only`: Only files and subdirectories whose path matches this regex will be transferred. (Note that `except` takes
  precedence over `only`. If both are supplied, a file or subdirectory matching both regexes will be excluded.)
* `immutable`: Files whose path matches this regex will be considered immutable, and `swift-http-import` will not check
  them for updates after having synced them once.

For `except` and `only`, you can distinguish between subdirectories and files because directory paths end with a slash,
whereas file paths don't.

For example, with the configuration below, directories called `sub_dir` and files with a `.gz` extension are excluded on
every level in the source tree:

```yaml
jobs:
  - from:
      url: http://de.archive.ubuntu.com/ubuntu/
    to:
      container: mirror
      object_prefix: ubuntu-repos
    except: "sub_dir/$|.gz$"
```

When using `only` to select files, you will usually want to include an alternative `/$` in the regex to match all
directories. Otherwise all directories will be excluded and you will only include files in the toplevel directory.

```yaml
jobs:
  - from:
      url: http://de.archive.ubuntu.com/ubuntu/
    to:
      container: mirror
      object_prefix: ubuntu-repos
    only: "/$|.amd64.deb$"
```

The `immutable` regex is especially useful for package repositories because package files, once uploaded, will never change:

```yaml
jobs:
  - from:
      url: http://de.archive.ubuntu.com/ubuntu/
    to:
      container: mirror
      object_prefix: ubuntu-repos
    immutable: '.*\.deb$'
```

### Transfer behavior: Segmenting

Swift rejects objects beyond a certain size (usually 5 GiB). To import larger files,
[segmenting](https://docs.openstack.org/swift/latest/overview_large_objects.html) must be used. The configuration
section `jobs[].segmenting` enables segmenting for the given job:

```yaml
jobs:
  - from:
      url: http://de.archive.ubuntu.com/ubuntu/
    to:
      container: mirror
      object_prefix: ubuntu-repos
    segmenting:
      min_bytes:     2147483648      # import files larger than 2 GiB...
      segment_bytes: 1073741824      # ...as segments of 1 GiB each...
      container:     mirror_segments # ...which are stored in this container (optional, see below)
```

Segmenting behaves like the standard `swift` CLI client with the `--use-slo` option:

- The segment container's name defaults to the target container's name plus a `_segments` prefix.
- Segments are uploaded with the object name `$OBJECT_PATH/slo/$UPLOAD_TIMESTAMP/$OBJECT_SIZE_BYTES/$SEGMENT_SIZE_BYTES/$SEGMENT_INDEX`.
- The target object uses an SLO manifest. DLO manifests are not supported.

### Transfer behavior: Expiring objects

Swift allows for files to be set to
[expire at a user-configured time](https://docs.openstack.org/swift/latest/overview_expiring_objects.html), at which
point they will be deleted automatically. When transfering files from a Swift source, `swift-http-import` will copy any
expiration dates to the target, unless the `jobs[].expiration.enabled` configuration option is set to `false`.

```yaml
jobs:
  - from:
      ... # not shown: credentials for Swift source
      container: source-container
    to:
      container: target-container
    expiration:
      enabled: false
```

In some cases, it may be desirable for target objects to live longer than source objects. For example, when syncing from
an on-site backup to an off-site backup, it may be useful to retain the off-site backup for a longer period of time than
the on-site backup. Use the `jobs[].expiration.delay_seconds` configuration option to shift all expiration dates on the
target side by a fixed amount of time compared to the source side.

```yaml
jobs:
  - from:
      ... # not shown: credentials for Swift source
      container: on-site-backup
    to:
      container: off-site-backup
    expiration:
      delay_seconds: 1209600 # retain off-site backups for 14 days longer than on-site backup
```

### Performance

By default, only a single worker thread will be transferring files. You can scale this up by including a `workers` section at the top level like so:

```yaml
workers:
  transfer: 10
```

## Log output

Log output on `stderr` is very sparse by default. Errors are always reported, and a final count will appear at the end like this:

```
2016/12/19 14:28:23 INFO: 103 dirs scanned, 0 failed
2016/12/19 14:28:23 INFO: 1496 files found, 167 transferred, 3 failed
```

In this case, all sources contain 103 directories and 1496 files. 170 files were found to be newer on the source and
thus need transfer. Of those, 167 were successfully transferred, and the remaining 3 file transfers failed (so there
should be 3 errors in the log).

## StatsD metrics

Adding an optional statsd config section enables submitting StatsD metrics.
```yaml
statsd:
  hostname: localhost
  port:     8125
  prefix:   swift_http_import
```

The following metric are sent:

| Kind    | Name                         | Description
| ------- | ---------------------------- | --------------------------------------------
| Gauge   | `last_run.success`           | `1` if no error occured, otherwise 0
| Gauge   | `last_run.success_timestamp` | UNIX timestamp of last succesful run
| Gauge   | `last_run.duration_seconds`  | Runtime in seconds
| Gauge   | `last_run.dirs_scanned`      | Number of directories scanned
| Gauge   | `last_run.files_found`       | Number of files found
| Gauge   | `last_run.files_transfered`  | Number of files actually transfered
| Gauge   | `last_run.files_failed`      | Number of files failed (download or upload)

# swift-http-import

* [Why this instead of rclone?](#why-this-instead-of-rclone)
* [Do NOT use if...](#do-not-use-if)
* [Implicit assumptions](#implicit-assumptions)
* [Installation](#installation)
* [Usage](#usage)
  * [Source specification](#source-specification)
  * [File selection](#file-selection)
    * [By name](#by-name)
    * [By age](#by-age)
    * [Simplistic file comparison](#simplistic-file-comparison)
  * [Transfer behavior: Segmenting on the source side](#transfer-behavior-segmenting-on-the-source-side)
  * [Transfer behavior: Segmenting on the target side](#transfer-behavior-segmenting-on-the-target-side)
  * [Transfer behavior: Expiring objects](#transfer-behavior-expiring-objects)
  * [Transfer behavior: Symlinks](#transfer-behavior-symlinks)
  * [Transfer behavior: Delete objects on the target side](#transfer-behavior-delete-objects-on-the-target-side)
  * [Performance](#performance)
* [Log output](#log-output)
* [StatsD metrics](#statsd-metrics)

This tool imports files from an HTTP server into a Swift container. Given an input URL, it recurses through the directory
listings on that URL, and mirrors all files that it finds into Swift. It will take advantage of `Last-Modified` and
`Etag` response headers to avoid repeated downloads of the same content, so best performance is ensured if the HTTP server
handles `If-Modified-Since` and `If-None-Match` request headers correctly.

## Why this instead of rclone?

[rclone](https://github.com/ncw/rclone/) is a similar tool that supports a much wider range of sources, and supports
targets other than Swift. Users who need to sync from or to different clouds might therefore prefer rclone. If your only
target is Swift, swift-http-import is better than rclone because it is built by people who operate Swift clusters for a
living and know all the intricacies of the system, and how to optimize Swift clients for maximum performance and
stability:

* rclone does not support Swift symlinks. Symlinks will be copied as regular files, thereby potentially wasting space on
  the target storage. swift-http-import recognizes symlinks and copies them as a symlink.
* rclone transfers large objects using the Dynamic Large Object strategy. DLO suffers from severe eventual-consistency
  problems when the Swift cluster is under high load. swift-http-import uses the much more resilient Static Large Object
  strategy instead.
* rclone does not propagate expiration dates when transferring between Swift containers. swift-http-import does.
* swift-http-import uses customized transfer strategies to ensure a stable transfer over narrow or flaky network
  connections.

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

Call with the path to a configuration file. The file should look like this:
[(Link to full example config file)](./examples/basic.yaml)

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

Instead of providing the Swift credential's password as plain text in the
config file, you can use the special syntax for the `password` field which will
tell `swift-http-import` to read the respective password from an exported
environment variable:

```yaml
password: { fromEnv: ENVIRONMENT_VARIABLE }
application_credential_secret: { fromEnv: ENVIRONMENT_VARIABLE }
```

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

In `jobs[].from`, you can pin the server's CA certificate, and specify a TLS client certificate (including private key)
that will be used by the HTTP client.
[(Link to full example config file)](./examples/source-clientcert.yaml)

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

#### Yum

If `jobs[].from.url` refers to a Yum repository (as used by most RPM-based Linux distributions), setting
`jobs[].from.type` to `yum` will cause `swift-http-import` to parse repository metadata to discover files to transfer,
instead of looking at directory listings.

*Warning:* With this option set, files below the given URL which are not
referenced by the Yum repository metadata will **not** be picked up.
Conversely, files mentioned in the repo metadata that don't exist in the repo
will result in `404` errors.

If the optional `jobs[].from.arch` field is given, the Yum repository metadata reader will only consider packages for
these architectures. Special values include "noarch" for architecture-independent packages and "src" for source
packages.

The GPG signature for the repository's metadata file is verified by default and
the job will be skipped if the verification is unsuccessful. This behavior can be disabled by
specifying the `jobs[].from.verify_signature` option.

[Link to full example config file](./examples/source-yum.yaml)

```yaml
jobs:
  - from:
      url:  https://dl.fedoraproject.org/pub/epel/7Server/x86_64/
      type: yum
      arch: [x86_64, noarch]
      verify_signature: true
      # SSL certs are optionally supported here, too
      cert: /path/to/client.pem
      key:  /path/to/client-key.pem
      ca:   /path/to/server-ca.pem
    to:
      container: mirror
      object_prefix: redhat/server/7/epel
```

#### Debian

If `jobs[].from.url` refers to a Debian repository (or an Ubuntu repository),
setting `jobs[].from.type` to `debian` will cause `swift-http-import` to parse
package and source metadata files to discover which respective files to
transfer, instead of looking at directory listings.

If the optional `jobs[].from.arch` field is given, the Debian repository
metadata reader will only consider package and source files for these
architectures.

*Warning:* If `arch` field is omitted then files mentioned in the repository
metadata that don't actually exist in the repository will result in `404`
errors.

The GPG signature for the repository's metadata file is verified by default and
the job will be skipped if the verification is unsuccessful. This behavior can be disabled by
specifying the `jobs[].from.verify_signature` option.

 [Link to full example config file](./examples/source-debian.yaml)

```yaml
jobs:
  - from:
      url:  http://de.archive.ubuntu.com/ubuntu/
      type: debian
      dist: [xenial, xenial-updates, disco, cosmic]
      arch: [amd64, i386]
      verify_signature: true
      # SSL certs are optionally supported here, too
      cert: /path/to/client.pem
      key:  /path/to/client-key.pem
      ca:   /path/to/server-ca.pem
    to:
      container: mirror
      object_prefix: ubuntu
```

#### Swift

Alternatively, the source in `jobs[].from` can also be a private Swift container if Swift credentials are specified
instead of a source URL.
[(Link to full example config file)](./examples/source-swift.yaml)

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

### File selection

#### By name

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
[(Link to full example config file)](./examples/filter-except.yaml)

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
[(Link to full example config file)](./examples/filter-only.yaml)

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
[(Link to full example config file)](./examples/filter-immutable.yaml)

```yaml
jobs:
  - from:
      url: http://de.archive.ubuntu.com/ubuntu/
    to:
      container: mirror
      object_prefix: ubuntu-repos
    immutable: '.*\.deb$'
```

#### By age

The `jobs[].match.not_older_than` configuration option can be used to exclude objects that are older than some
threshold, as indicated by the `Last-Modified` header of the source object.
[(Link to full example config file)](./examples/filter-not-older-than.yaml)

```yaml
jobs:
  - from:
      ... # not shown: credentials for Swift source
      container: on-site-backup
    to:
      container: off-site-backup
    match:
      not_older_than: 3 days # ignore old backups, focus on the recent ones
```

The value of `jobs[].match.not_older_than` is a value with one of the following units:

- `seconds` (`s`)
- `minutes` (`m`)
- `hours` (`h`)
- `days` (`d`)
- `weeks` (`w`)

*Warning:* As of this version, this configuration option only works with Swift sources.


#### Simplistic file comparison

The `jobs[].match.simplistic_comparison` configuration option can be used to
force `swift-http-import` to only use last modified time to determine file
transfer eligibility. This option can be used for compatibility with other
similar tools (e.g. `rclone`). Without it, `swift-http-import` is likely to
retransfer files that were already transferred by other tools due to its strict
comparison constraints.

**Note**: This option is only _99.9% reliable_, if you need 100% reliability
for file transfer eligibility then you should not use this option and let
`swift-http-import` default to its strong comparison constraints.

[(Link to full example config file)](./examples/filter-simplistic-comparison.yaml)

```yaml
jobs:
  - from:
      ...
    to:
      container: off-site-backup
    match:
      simplistic_comparison: true
```


### Transfer behavior: Segmenting on the source side

By default, `swift-http-import` will download source files in segments of at most 500 MiB, using [HTTP range
requests](https://tools.ietf.org/html/rfc7233). Range requests are supported by most HTTP servers that serve static
files, and servers without support will fallback to regular HTTP and send the whole file at once. **Note that** range
requests are currently not supported for Swift sources that require authentication.

In the unlikely event that range requests confuse the HTTP server at the source side, they can be disabled by setting
`jobs[].from.segmenting` to `false`:
[(Link to full example config file)](./examples/transfer-no-source-segmenting.yaml)

```yaml
jobs:
  - from:
      url: http://de.archive.ubuntu.com/ubuntu/
      segmenting: false
    to:
      container: mirror
      object_prefix: ubuntu-repos
```

The default segment size of 500 MiB can be changed by setting `jobs[].from.segment_bytes` like so:
[(Link to full example config file)](./examples/transfer-source-segmenting.yaml)

```yaml
jobs:
  - from:
      url: http://de.archive.ubuntu.com/ubuntu/
      segment_bytes: 1073741824 # 1 GiB
    to:
      container: mirror
      object_prefix: ubuntu-repos
```

### Transfer behavior: Segmenting on the target side

Swift rejects objects beyond a certain size (usually 5 GiB). To import larger files,
[segmenting](https://docs.openstack.org/swift/latest/overview_large_objects.html) must be used. The configuration
section `jobs[].segmenting` enables segmenting for the given job:
[(Link to full example config file)](./examples/transfer-target-segmenting.yaml)

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
point they will be deleted automatically. When transferring files from a Swift source, `swift-http-import` will copy any
expiration dates to the target, unless the `jobs[].expiration.enabled` configuration option is set to `false`.
[(Link to full example config file)](./examples/transfer-no-expiring.yaml)

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
[(Link to full example config file)](./examples/transfer-expiring.yaml)

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

### Transfer behavior: Symlinks

Starting with the Queens release, Swift optionally supports symlinks. A symlink is a light-weight reference to some
other object (possibly in a different container and/or account). When a HEAD or GET request is sent to retrieve the
symlink's metadata or content, the linked object's metadata or content is returned instead.

When swift-http-import transfers from a Swift source (i.e., Swift credentials are given in `jobs[].from`), and when the
target side supports symlinks, symlinks in the source side will be copied as symlinks. No extra configuration is
necessary for this behavior.

However, the link target must be transferred **in the same job**. (This restriction may be lifted in a later version.)
Otherwise, the symlink will be transferred as a regular object, possibly resulting in duplication of file contents on
the target side.

### Transfer behavior: Delete objects on the target side

By default, swift-http-import will only copy files from the source side to the target side. To enable the deletion of
objects that exist on the target side, but not on the source side, set the `jobs[].cleanup.strategy` configuration
option to `delete`.
[(Link to full example config file)](./examples/transfer-delete-on-target.yaml)

```yaml
jobs:
  - from:
      url: http://de.archive.ubuntu.com/ubuntu/
    to:
      container: mirror
      object_prefix: ubuntu-repos
    cleanup:
      strategy: delete
```

Another possible value for `jobs[].cleanup.strategy` is `report`, which will log objects that `delete` would clean
up without actually touching them.

When combined with `jobs[].only` and/or `jobs[].except`, cleanup will delete all files excluded by those filters, even
if the same file exists on the source side. This is the same behavior as if `--delete-excluded` is given to rsync.

### Performance

By default, only a single worker thread will be transferring files. You can scale this up by including a `workers` section at the top level like so:
[(Link to full example config file)](./examples/basic-multiple-workers.yaml)

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

When the environment variable `LOG_TRANSFERS=true` is set, a log line will be printed for each transferred file:

```
2016/12/19 14:28:19 INFO: transferring to container-name/path/to/object
```

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
| Gauge   | `last_run.success`           | `1` if no error occurred, otherwise 0
| Gauge   | `last_run.success_timestamp` | UNIX timestamp of last successful run
| Gauge   | `last_run.duration_seconds`  | Runtime in seconds
| Gauge   | `last_run.jobs_skipped`      | Number of jobs skipped
| Gauge   | `last_run.dirs_scanned`      | Number of directories scanned
| Gauge   | `last_run.files_found`       | Number of files found
| Gauge   | `last_run.files_transfered`  | Number of files actually transferred
| Gauge   | `last_run.files_failed`      | Number of files failed (download or upload)
| Gauge   | `last_run.bytes_transfered`  | Number of bytes transferred

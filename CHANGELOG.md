## v2.10.0 - 2023-02-28

New features:
- Add support for specifying TLS client certificate and key file.
- Add `{ fromEnv: ENV_VAR }` syntax support for all Swift options.

Changes:
- Updated all dependencies to their latest versions.

## v2.9.1 - 2022-05-25

Bugfixes:
- Fixed escaping of characters in the file path of GitHub releases.

Changes:
- GitHub release assets are now mirrored under its tag name instead of
  `releases/download/$tagName/`.

## v2.9.0 - 2022-05-19

New features:
- Add support for GitHub releases.

Changes:
- Updated all dependencies to their latest versions.

Bugfixes:
- Fixed transfer of objects from swift source when object name is not a well-formed path. For example, an object name like "a///b" is not wrongly normalized into "a/b" anymore.

## v2.8.0 - 2021-08-11

New features:
- Add support for selecting GPG keyservers using the `gpg.keyserver_urls` config option.
- Add support for caching GPG public keys to a Swift container (`gpg.cache_container_name`). The keys will be loaded
  into memory on startup in order to avoid downloading the same keys every time.

Changes:
- Since `pool.sks-keyservers.net` has been discontinued, GPG keys are now retrieved from `keyserver.ubuntu.com` and
  `pgp.mit.edu` by default.
- Files for large Swift containers are now transferred in a streaming manner. This results in a performance increase as
  `swift-http-import` doesn't have to wait for a full list of files before any transfer jobs can be enqueued.
- When scraping fails, the cleanup phase is now skipped for the respective job to avoid cleaning up too much by mistake.

## v2.7.0 - 2021-05-31

New features:
- If specifying the user password in the config file is not desired, [application credentials][app-cred] can now be used
  instead, both for the `swift` section and in the `jobs[].from` sections.

  ```yaml
  swift:
    auth_url: https://my.keystone.local:5000/v3
    application_credential_id: 80e810bf385949ae8f0e251f90269515
    application_credential_secret: eixohMoo1on5ohng

  jobs: ...
  ```

[app-cred]: https://docs.openstack.org/python-openstackclient/latest/cli/command-objects/application-credentials.html

Changes:
- All dependencies have been upgraded to their latest versions.
- For source type `yum`, add support for XZ-compressed repositories (previous versions only supported GZip).

Bugfixes:
- For source type `swift`, fix detection of pseudo-directories that are located at the root of a job's search space
  (i.e.  having the same name as the `object_prefix`).

## v2.6.0 - 2019-11-27

New features:
- Report the total number of bytes transferred per run.
- The GPG signatures for `yum` and `debian` source types are verified by
  default. This behaviour can be disabled using the new config option: `jobs[].from.verify_signature`.
- The new `jobs[].match.not_older_than` configuration option can be used to exclude old objects from transfer. As of
  now, it can only be used with Swift sources, not with HTTP sources.
- When syncing a Debian (Ubuntu) repository, the `jobs[].from.type` may be set
  to `debian` to instruct `swift-http-import` to parse the source and package
  metadata files instead of the HTTP server's directory listings to find which
  package and source files to transfer.
- For better compatibility with other similar tools (e.g. `rclone`), the
  `jobs[].match.simplistic_comparison` configuration option can be used which
  will allow `swift-http-import` to use less metadata for determining file
  transfer eligibility.
- Swift credential passwords can be read from exported environment variables
  instead of providing them in the config file by using the syntax:

    ```yaml
    password: { fromEnv: ENVIRONMENT_VARIABLE }
    ```

## v2.5.0 - 2018-09-27

Changes:
- Improve performance with Swift sources by listing all source objects in one sweep. Previously, the same strategy as
  for HTTP sources was employed, where each directory is listed separately.
- Retroactively change version numbers `vX.Y` to `vX.Y.0` to achieve full compliance with the
  [SemVer 2.0.0 spec](https://semver.org/spec/v2.0.0.html).
- Improve log format for skipping decisions to always show which path was rejected by the inclusion/exclusion regexes.
  For example:

    Old:
    ```
    DEBUG: skipping /files/1.txt: is not included by `[0-9].txt`
    ```

    New:
    ```
    DEBUG: skipping /files/1.txt: /files/ is not included by `[0-9].txt`
    ```

Bugfixes:
- When using a Swift source, pseudo-directories are now recognized and transferred correctly.
- When uploading a segmented object to the target, expiration dates are now also applied to the segments.
  If you used an older version of `swift-http-import` to transfer files with expiration dates using segmented uploading,
  you will have to clean up those segments manually once the objects themselves have expired.

## v2.4.0 - 2018-06-14

New features:
- `swift-http-import` can now clean up objects on the target side that have been deleted on the source side. To enable
  this behavior, set the new `jobs[].cleanup.strategy` configuration option to `delete`. Or set it to `report` to report
  such objects without deleting them.
- Initial support for Swift symlinks has been added. When a Swift source contains a object that is a symlink to another
  object, the object is also uploaded as a symlink on the target side, thus avoiding duplicate transfers of identical
  files. In this version, only those symlinks are considered that point to objects which are transferred in the same
  job. A future version may improve this to allow symlinks to also point to objects transferred in a different job.

Changes:
- Switch the Swift backend from [ncw/swift](https://github.com/ncw/swift) to
  [Schwift](https://github.com/majewsky/schwift). This is important to facilitate some of the new features above.
- When deleting a file on the target side (after an upload error), do not log an error if the DELETE request
  returns 404 (Not Found).

Bugfixes:
- When an SLO on the target side is being overwritten with a regular non-segmented object, `swift-http-import` now
  correctly deletes the SLO's segments.

## v2.3.0 - 2018-01-29

New features:
- When `--version` is given, the release version is reported on standard output.
- When `jobs[].from` refers to a URL source, and the server for that URL supports HTTP Range Requests, files are now
  downloaded in segments of 500 MiB to avoid overly long connections. Furthermore, if a segmented download fails,
  `swift-http-import` is now able to restart the download without having to download the entire file again. Segmented
  downloading can be disabled and the segment size can be changed in the new `jobs[].from.segmenting` configuration
  section.

Changes:
- When making HTTP requests, the correct User-Agent `swift-http-import/x.y.z` is now reported.

## v2.2.1 - 2018-01-15

Bugfixes:
- An issue was fixed where file state was not correctly tracked for large objects, which caused large objects to be
  mirrored on every run even when the target was already up-to-date.

## v2.2.0 - 2017-12-07

New features:
- When syncing a Yum repository, the `jobs[].from.type` may be set to `"yum"` to instruct `swift-http-import` to parse
  the repository metadata instead of the HTTP server's directory listings to find which files to transfer. Note that any
  files below the repository URL which are not referenced in the repository metadata will not be transferred.
- When syncing a Yum repository like described above, the `repodata/repomd.xml` will be downloaded first, but uploaded
  last. This ensures that (barring unexpected transfer errors) clients using the target repository will never observe it
  in an inconsistent state, i.e., metadata will only start referencing packages once they have been transferred.

## v2.1.0 - 2017-11-16

New features:
- If the environment variable `LOG_TRANSFERS=true` is given, transferred files will now be logged as they are being transferred.
  Logging only occurs if the file is actually transferred, not if the target is found to be up-to-date.

Changes:
- Giving an invalid URL in `jobs[].from.url` now results in immediate failure during configuration parsing instead of
  indeterministic errors later on.
- It is now an error for `jobs[].from.url` to not have a trailing slash. For now, a missing trailing slash will be added
  and execution will continue, but this error will become fatal in a future version.
- The README now includes anti-usecases, in the "Do NOT use if..." section.

Bugfixes:
- Percent-encoded URLs in directory listings are now decoded correctly.
- An issue was fixed where the immutability regex was not always respected for large containers.

## v2.0.0 - 2017-10-16

**Backwards-incompatible changes:**
- The configuration format has changed slightly to be more consistent with itself.
  Refer to the README for details. The following things have changed in particular:
  - `jobs[].from` becomes `jobs[].from.url` (unless it is an object already).
  - `jobs[].to` is split into `jobs[].to.container` and `jobs[].to.object_prefix`.
  - `jobs[].{ca,cert,key}` move into `jobs[].from`.

New features:
- `swift-http-import` can now transfer large objects by using the Static Large Object method of Swift. The
  `jobs[].segmenting` configuration section must be specified to enable segmenting.
- When transferring files from a Swift source, `swift-http-import` will now recognize objects with an expiry timestamp, and
  mirror the expiry timestamp to the target side. The `jobs[].expiration` configuration section can be used to control
  this behavior.

Changes:
- The code has been restructured for better extensibility and high-level readability.
- The README has been restructured to be less chaotic, and a TOC has been added for better discoverability.

Bugfixes:
- Interrupts (SIGTERM and SIGINT) are now ignored less often.

## v1.1.0 - 2017-08-21

New features:
- Add a simple retry logic:
  - A failed directory listing will be postponed and retried up to two times at the end of scraping.
  - A failed file transfer will be postponed and retried once when all other transfers have completed.
- Report number of failed directory listings.

Changes:
- Exit with non-zero status when any directory listing or file transfer fails.

Bugfixes:
- Report failure when a source file cannot be retrieved (instead of uploading the error message to the target).

## v1.0.0 - 2017-08-18

Initial release.

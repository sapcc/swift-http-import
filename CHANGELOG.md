# v2.3.1 (TBD)

Changes:
- When deleting a file on the target side (usually after an upload error), do not log an error if the DELETE request
  returns 404 (Not Found).

Bugfixes:
- When an SLO on the target side is being overwritten with a regular non-segmented object, swift-http-import now
  correctly deletes the SLO's segments.

# v2.3 (2018-01-29)

New features:
- When `--version` is given, the release version is reported on standard output.
- When `jobs[].from` refers to a URL source, and the server for that URL supports HTTP Range Requests, files are now
  downloaded in segments of 500 MiB to avoid overly long connections. Furthermore, if a segmented download fails,
  swift-http-import is now able to restart the download without having to download the entire file again. Segmented
  downloading can be disabled and the segment size can be changed in the new `jobs[].from.segmenting` configuration
  section.

Changes:
- When making HTTP requests, the correct User-Agent "swift-http-import/x.y.z" is now reported.

# v2.2.1 (2018-01-15)

Bugfixes:
- An issue was fixed where file state was not correctly tracked for large objects, which caused large objects to be
  mirrored on every run even when the target was already up-to-date.

# v2.2 (2017-12-07)

New features:
- When syncing a Yum repository, the `jobs[].from.type` may be set to `"yum"` to instruct `swift-http-import` to parse
  the repository metadata instead of the HTTP server's directory listings to find which files to transfer. Note that any
  files below the repository URL which are not referenced in the repository metadata will not be transferred.
- When syncing a Yum repository like described above, the `repodata/repomd.xml` will be downloaded first, but uploaded
  last. This ensures that (barring unexpected transfer errors) clients using the target repository will never observe it
  in an inconsistent state, i.e., metadata will only start referencing packages once they have been transferred.

# v2.1 (2017-11-16)

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

# v2.0 (2017-10-16)

**Backwards-incompatible changes:**
- The configuration format has changed slightly to be more consistent with itself.
  Refer to the README for details. The following things have changed in particular:
  - `jobs[].from` becomes `jobs[].from.url` (unless it is an object already).
  - `jobs[].to` is split into `jobs[].to.container` and `jobs[].to.object_prefix`.
  - `jobs[].{ca,cert,key}` move into `jobs[].from`.

New features:
- swift-http-import can now transfer large objects by using the Static Large Object method of Swift. The
  `jobs[].segmenting` configuration section must be specified to enable segmenting.
- When transfering files from a Swift source, swift-http-import will now recognize objects with an expiry timestamp, and
  mirror the expiry timestamp to the target side. The `jobs[].expiration` configuration section can be used to control
  this behavior.

Changes:
- The code has been restructured for better extensibility and high-level readability.
- The README has been restructured to be less chaotic, and a TOC has been added for better discoverability.

Bugfixes:
- Interrupts (SIGTERM and SIGINT) are now ignored less often.

# v1.1 (2017-08-21)

New features:
- Add a simple retry logic:
  - A failed directory listing will be postponed and retried up to two times at the end of scraping.
  - A failed file transfer will be postponed and retried once when all other transfers have completed.
- Report number of failed directory listings.

Changes:
- Exit with non-zero status when any directory listing or file transfer fails.

Bugfixes:
- Report failure when a source file cannot be retrieved (instead of uploading the error message to the target).

# v1.0 (2017-08-18)

Initial release.
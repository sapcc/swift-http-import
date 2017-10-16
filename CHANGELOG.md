# v1.1 (2017-08-21)

Changes:
- Add a simple retry logic:
  - A failed directory listing will be postponed and retried up to two times at the end of scraping.
  - A failed file transfer will be postponed and retried once when all other transfers have completed.
- Exit with non-zero status when any directory listing or file transfer fails.
- Report number of failed directory listings.

Bugfixes:
- Report failure when a source file cannot be retrieved (instead of uploading the error message to the target).

# v1.0 (2017-08-18)

Initial release.

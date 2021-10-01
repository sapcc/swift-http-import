#!/usr/bin/env bash
set -euo pipefail

if [[ ! -v LIB_SOURCED ]]; then
  export SOURCE_TYPE=http
  cd "$(readlink -f "$(dirname "$0")")/.."
  # shellcheck disable=SC1090,SC1091
  source lib.sh
fi

step 'Test 10: Chunked download'

# This test specifically checks that segmented upload works correctly when a file is
# downloaded segmentedly. There was a bug where EnhancedGet() reported the
# Content-Length of the first segment only (instead of the whole file), causing
# the segmenting logic to incorrectly determine when to upload as a large object.

upload_file_from_stdin just/some/files/1.txt <<-EOF
  Hello World.
EOF
upload_file_from_stdin just/some/files/2.txt <<-EOF
  Hello Second World.
EOF
upload_file_from_stdin largefile.txt <<-EOF
  Line number 1
  Line number 2
  Line number 3
  Line number 4
  Line number 5
EOF
upload_file_from_stdin just/another/file.txt <<-EOF
  This is the new file!
EOF
upload_file_from_stdin expires.txt -H 'X-Delete-At: 2000000000' <<-EOF
  This will expire soon.
EOF

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from:
        url: ${SOURCE_URL}/
        segment_bytes: 20 # less than job.segmenting.min_bytes, but also more
                          # than the smallest files (to exercise all code paths)
      to: { container: ${CONTAINER_BASE}-test10 }
      except: 'expires-with-segments.txt'
      segmenting:
        container: ${CONTAINER_BASE}-test10-segments
        min_bytes: 30
        segment_bytes: 14
EOF
# NOTE: A segment size of 14 bytes should put each line of text in its own
# segment, i.e. 5 segments.

expect test10 <<-EOF
>> expires.txt
This will expire soon.
>> just/another/file.txt
This is the new file!
>> just/some/files/1.txt
Hello World.
>> just/some/files/2.txt
Hello Second World.
>> largefile.txt
Line number 1
Line number 2
Line number 3
Line number 4
Line number 5
EOF

SEGMENT_COUNT="$(swift list "${CONTAINER_BASE}-test10-segments" | wc -l)"
if [ "${SEGMENT_COUNT}" -ne 5 ]; then
  printf "\e[1;31m>>\e[0;31m Expected SLO to have 5 segments, but got %s instead:\e[0m\n" "$SEGMENT_COUNT"
  dump test10-segments
  exit 1
fi

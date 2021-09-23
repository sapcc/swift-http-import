#!/usr/bin/env bash

step 'Test 10-chunked-download'

# This test specifically checks that segmented upload works correctly when a file is
# downloaded segmentedly. There was a bug where EnhancedGet() reported the
# Content-Length of the first segment only (instead of the whole file), causing
# the segmenting logic to incorrectly determine when to upload as a large object.

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from:
        url: ${SOURCE_URL}/
        segment_bytes: 20 # less than job.segmenting.min_bytes, but also more
                          # than the smallest files (to exercise all code paths)
      to: { container: ${CONTAINER_BASE}-test8 }
      except: 'expires-with-segments.txt'
      segmenting:
        container: ${CONTAINER_BASE}-test8-segments
        min_bytes: 30
        segment_bytes: 14
EOF
# NOTE: A segment size of 14 bytes should put each line of text in its own
# segment, i.e. 5 segments.

expect test8 <<-EOF
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
CHANGED
EOF

SEGMENT_COUNT="$(swift list "${CONTAINER_BASE}-test8-segments" | wc -l)"
if [ "${SEGMENT_COUNT}" -ne 5 ]; then
  printf "\e[1;31m>>\e[0;31m Expected SLO to have 5 segments, but got %s instead:\e[0m\n" "$SEGMENT_COUNT"
  dump test8-segments
  exit 1
fi

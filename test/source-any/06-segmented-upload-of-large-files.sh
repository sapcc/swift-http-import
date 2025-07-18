#!/usr/bin/env bash

# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

if [[ ! -v LIB_SOURCED ]]; then
  cd "$(readlink -f "$(dirname "$0")")/.."
  # shellcheck disable=SC1090,SC1091
  source lib.sh
  setup "$@"
fi

step 'Test 06: Segmented upload of large files'

upload_file_from_stdin just/some/files/1.txt <<-EOF
  Hello World.
EOF
upload_file_from_stdin just/some/files/2.txt <<-EOF
  Hello Second World.
EOF
upload_file_from_stdin just/another/file.txt <<-EOF
  This is the new file!
EOF
upload_file_from_stdin largefile.txt <<-EOF
  Line number 1
  Line number 2
  Line number 3
  Line number 4
  Line number 5
EOF

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${SOURCE_SPEC}
      to: { container: ${CONTAINER_BASE}-test6 }
      segmenting:
        container: ${CONTAINER_BASE}-test6-segments
        min_bytes: 30
        segment_bytes: 14
EOF
# NOTE: A segment size of 14 bytes should put each line of text in its own
# segment, i.e. 5 segments.

expect test6 <<-EOF
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

SEGMENT_COUNT="$(swift list "${CONTAINER_BASE}-test6-segments" | wc -l)"
if [[ $SEGMENT_COUNT -ne 5 ]]; then
  printf "\e[1;31m>>\e[0;31m Expected SLO to have 5 segments, but got %s instead:\e[0m\n" "$SEGMENT_COUNT"
  swift list "${CONTAINER_BASE}-test6-segments" | sed 's/^/    /'
  exit 1
fi
# NOTE: This also ensures that the small files are not uploaded in segments,
# because then the segment count would be much more than 5.

################################################################################

step 'Test 06 (cont.): Overwrite SLO on target with non-segmented object'

upload_file_from_stdin largefile.txt <<-EOF
  Line number 1
  Line number 2
  Line number 3
  Line number 4
  CHANGED
EOF

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${SOURCE_SPEC}
      to: { container: ${CONTAINER_BASE}-test6 }
      only: largefile.txt
EOF

expect test6 <<-EOF
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

# check that segments have been cleaned up, i.e. segment container should be empty
expect test6-segments </dev/null

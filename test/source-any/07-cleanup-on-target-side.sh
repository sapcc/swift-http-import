#!/usr/bin/env bash
set -euo pipefail

if [[ ! -v LIB_SOURCED ]]; then
  cd "$(readlink -f "$(dirname "$0")")/.."
  # shellcheck disable=SC1090,SC1091
  source lib.sh
fi

step 'Test 07-cleanup-on-target-side'

upload_file_from_stdin just/some/files/1.txt <<-EOF
  Hello World.
EOF

upload_target_file_from_stdin test07 ignored.txt <<-EOF
  This file does not get cleaned up because it's not below object_prefix.
EOF
upload_target_file_from_stdin test07 target/cleanup-please.txt <<-EOF
  This file gets cleaned up because it's below object_prefix.
EOF
# This file will get cleaned up even though it exists on the source side
# because it's excluded from transfer by a filter.
upload_target_file_from_stdin test07 target/just/some/files/2.txt <<-EOF
  Hello Second World.
EOF
sleep "$SLEEP" # wait for container listing to get updated

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${SOURCE_SPEC}
      to:
        container: ${CONTAINER_BASE}-test07
        object_prefix: target/
      only: '/$|1\\.txt$'
EOF

# This first pass does not have cleanup enabled (to compare against the second
# pass down below), so we're still seeing the files that need to be cleaned up.
expect test07 <<-EOF
>> ignored.txt
This file does not get cleaned up because it's not below object_prefix.
>> target/cleanup-please.txt
This file gets cleaned up because it's below object_prefix.
>> target/just/some/files/1.txt
Hello World.
>> target/just/some/files/2.txt
Hello Second World.
EOF

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${SOURCE_SPEC}
      to:
        container: ${CONTAINER_BASE}-test07
        object_prefix: target/
      only: '/$|1\\.txt$'
      cleanup:
        strategy: delete
EOF

expect test07 <<-EOF
>> ignored.txt
This file does not get cleaned up because it's not below object_prefix.
>> target/just/some/files/1.txt
Hello World.
EOF

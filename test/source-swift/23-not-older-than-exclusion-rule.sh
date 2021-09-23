#!/usr/bin/env bash
set -euo pipefail

if [[ ! -v LIB_SOURCED ]]; then
  export SOURCE_TYPE=swift
  cd "$(readlink -f "$(dirname "$0")")/.."
  # shellcheck disable=SC1090,SC1091
  source lib.sh
fi

step 'Test 23: "Not older than" exclusion rule'

# reset Last-Modified timestamp on this one file
upload_file_from_stdin just/some/files/2.txt <<-EOF
  Hello Second World.
EOF

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${SOURCE_SPEC}
      to:
        container: ${CONTAINER_BASE}-test23
      only: '^just/$|some/'
      match:
        not_older_than: 30 seconds
EOF

expect test23 <<-EOF
>> just/some/files/2.txt
Hello Second World.
EOF

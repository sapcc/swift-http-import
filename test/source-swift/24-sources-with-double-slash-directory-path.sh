#!/usr/bin/env bash
set -euo pipefail

if [[ ! -v LIB_SOURCED ]]; then
  export SOURCE_TYPE=swift
  cd "$(readlink -f "$(dirname "$0")")/.."
  # shellcheck disable=SC1090,SC1091
  source lib.sh
  setup "$@"
fi

step 'Test 24: Swift sources with double slash directory path'

upload_file_from_stdin pseudo//directory/file.txt <<-EOF
  Hello File.
EOF

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${SOURCE_SPEC}
      to:
        container: ${CONTAINER_BASE}-test24
      only: pseudo/
EOF

expect test24 <<-EOF
>> pseudo//directory/file.txt
Hello File.
EOF

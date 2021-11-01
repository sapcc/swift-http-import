#!/usr/bin/env bash
set -euo pipefail

if [[ ! -v LIB_SOURCED ]]; then
  export SOURCE_TYPE=swift
  cd "$(readlink -f "$(dirname "$0")")/.."
  # shellcheck disable=SC1090,SC1091
  source lib.sh
  setup "$@"
fi

step 'Test 22: Swift sources with pseudo-directories'

upload_file_from_stdin pseudo/directory/ </dev/null
upload_file_from_stdin pseudo/regularfile.txt <<-EOF
  Hello File.
EOF

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${SOURCE_SPEC}
      to:
        container: ${CONTAINER_BASE}-test22
      only: pseudo/
EOF

expect test22 <<-EOF
>> pseudo/directory/
>> pseudo/regularfile.txt
Hello File.
EOF

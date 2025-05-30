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

step 'Test 05: Immutability regex blocks re-transfer'

upload_file_from_stdin just/another/file.txt <<-EOF
  Hello Another World.
EOF
upload_file_from_stdin just/some/files/1.txt <<-EOF
  Hello World.
EOF

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${SOURCE_SPEC}
      to: { container: ${CONTAINER_BASE}-test5 }
      immutable: '.*.txt'
EOF

expect test5 <<-EOF
>> just/another/file.txt
Hello Another World.
>> just/some/files/1.txt
Hello World.
EOF

upload_file_from_stdin just/another/file.txt <<-EOF
This is the new file!
EOF
upload_file_from_stdin just/some/files/1.txt <<-EOF
  Hello changed World.
EOF

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${SOURCE_SPEC}
      to: { container: ${CONTAINER_BASE}-test5 }
      immutable: '.*.txt'
EOF

expect test5 <<-EOF
>> just/another/file.txt
Hello Another World.
>> just/some/files/1.txt
Hello World.
EOF

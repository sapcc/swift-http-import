#!/usr/bin/env bash
set -euo pipefail

if [[ ! -v LIB_SOURCED ]]; then
  cd "$(readlink -f "$(dirname "$0")")/.."
  # shellcheck disable=SC1090,SC1091
  source lib.sh
fi

step 'Test 04: Exclusion takes precedence over inclusion'

upload_file_from_stdin just/some/files/1.txt <<-EOF
  Hello World.
EOF
upload_file_from_stdin just/some/files/2.txt <<-EOF
  Hello Second World.
EOF
upload_file_from_stdin just/another/file.txt <<-EOF
  Hello Another World.
EOF

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${SOURCE_SPEC}
      to: { container: ${CONTAINER_BASE}-test4 }
      only: '/$|[0-9].txt'
      except: '2'
EOF

expect test4 <<-EOF
>> just/some/files/1.txt
Hello World.
EOF

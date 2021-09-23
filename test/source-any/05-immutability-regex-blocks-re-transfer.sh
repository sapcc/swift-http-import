#!/usr/bin/env bash

step 'Test 05-immutability-regex-blocks-re-transfer'

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${SOURCE_SPEC}
      to: { container: ${CONTAINER_BASE}-test5 }
      only: '/$|file.txt'
      immutable: '.*.txt'
EOF

expect test5 <<-EOF
>> just/another/file.txt
Hello Another World.
EOF

upload_file_from_stdin just/another/file.txt <<-EOF
  This is the new file!
EOF

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${SOURCE_SPEC}
      to: { container: ${CONTAINER_BASE}-test5 }
      only: '/$|file.txt'
      immutable: '.*.txt'
EOF

expect test5 <<-EOF
>> just/another/file.txt
Hello Another World.
EOF

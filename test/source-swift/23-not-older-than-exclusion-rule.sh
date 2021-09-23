#!/usr/bin/env bash

step 'Test 23-not-older-than-exclusion-rule'

# reset Last-Modified timestamp on this one file
upload_file_from_stdin just/some/files/2.txt <<-EOF
  Hello Second World.
EOF

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${SOURCE_SPEC}
      to:
        container: ${CONTAINER_BASE}-test12
      only: '^just/$|some/'
      match:
        not_older_than: 30 seconds
EOF

expect test12 <<-EOF
>> just/some/files/2.txt
Hello Second World.
EOF

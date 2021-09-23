#!/usr/bin/env bash

step 'Test 03-exclusion-regex'

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${SOURCE_SPEC}
      to: { container: ${CONTAINER_BASE}-test3 }
      only: '[0-9].txt'
EOF

expect test3 </dev/null # empty because the inclusion regex did not match the directories along the path

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${SOURCE_SPEC}
      to: { container: ${CONTAINER_BASE}-test3 }
      only: '/$|[0-9].txt'
EOF

expect test3 <<-EOF
>> just/some/files/1.txt
Hello World.
>> just/some/files/2.txt
Hello Second World.
EOF

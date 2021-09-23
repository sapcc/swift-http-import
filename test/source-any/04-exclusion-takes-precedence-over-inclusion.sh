#!/usr/bin/env bash

step 'Test 04-exclusion-takes-precedence-over-inclusion'

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

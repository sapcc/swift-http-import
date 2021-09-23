#!/usr/bin/env bash

step 'Test 02-exclusion-regex'

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${SOURCE_SPEC}
      to: { container: ${CONTAINER_BASE}-test2 }
      except: 'some/'
EOF

expect test2 <<-EOF
>> just/another/file.txt
Hello Another World.
EOF

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${SOURCE_SPEC}
      to: { container: ${CONTAINER_BASE}-test2 }
      except: '2'
EOF

expect test2 <<-EOF
>> just/another/file.txt
Hello Another World.
>> just/some/files/1.txt
Hello World.
EOF

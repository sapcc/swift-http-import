#!/usr/bin/env bash

step 'Test 01-mirror-from-http'

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${SOURCE_SPEC}
      to: { container: ${CONTAINER_BASE}-test1 }
EOF

expect test1 <<-EOF
>> just/some/files/1.txt
Hello World.
>> just/some/files/2.txt
Hello Second World.
EOF

################################################################################

step 'Test 01-mirror-from-http'

upload_file_from_stdin just/another/file.txt <<-EOF
  Hello Another World.
EOF
sleep "$SLEEP" # wait for container listing to get updated

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${SOURCE_SPEC}
      to: { container: ${CONTAINER_BASE}-test1 }
EOF

expect test1 <<-EOF
>> just/another/file.txt
Hello Another World.
>> just/some/files/1.txt
Hello World.
>> just/some/files/2.txt
Hello Second World.
EOF

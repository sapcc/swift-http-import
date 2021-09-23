#!/usr/bin/env bash

step 'Test 07-cleanup-on-target-side'

upload_target_file_from_stdin test10 ignored.txt <<-EOF
  This file does not get cleaned up because it's not below object_prefix.
EOF
upload_target_file_from_stdin test10 target/cleanup-please.txt <<-EOF
  This file gets cleaned up because it's below object_prefix.
EOF
# This file will get cleaned up even though it exists on the source side
# because it's excluded from transfer by a filter.
upload_target_file_from_stdin test10 target/just/some/files/2.txt <<-EOF
  Hello Second World.
EOF
sleep "$SLEEP" # wait for container listing to get updated

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${SOURCE_SPEC}
      to:
        container: ${CONTAINER_BASE}-test10
        object_prefix: target/
      only: '/$|1\\.txt$'
EOF

# This first pass does not have cleanup enabled (to compare against the second
# pass down below), so we're still seeing the files that need to be cleaned up.
expect test10 <<-EOF
>> ignored.txt
This file does not get cleaned up because it's not below object_prefix.
>> target/cleanup-please.txt
This file gets cleaned up because it's below object_prefix.
>> target/just/some/files/1.txt
Hello World.
>> target/just/some/files/2.txt
Hello Second World.
EOF

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${SOURCE_SPEC}
      to:
        container: ${CONTAINER_BASE}-test10
        object_prefix: target/
      only: '/$|1\\.txt$'
      cleanup:
        strategy: delete
EOF

expect test10 <<-EOF
>> ignored.txt
This file does not get cleaned up because it's not below object_prefix.
>> target/just/some/files/1.txt
Hello World.
EOF

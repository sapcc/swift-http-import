#!/usr/bin/env bash

step 'Test 22-sources-with-pseudo-directories'

upload_file_from_stdin pseudo/directory/ </dev/null
upload_file_from_stdin pseudo/regularfile.txt <<-EOF
  Hello File.
EOF

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${SOURCE_SPEC}
      to:
        container: ${CONTAINER_BASE}-test11
      only: pseudo/
EOF

expect test11 <<-EOF
>> pseudo/directory/
>> pseudo/regularfile.txt
Hello File.
EOF

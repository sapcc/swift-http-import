#!/usr/bin/env bash

step 'Test 21-symlinks'

# Uploading a symlink requires curl because python-swiftclient has not catched up with Swift yet.
curl -H "X-Auth-Token: ${OS_AUTH_TOKEN}" -X PUT -d '' -H "Content-Type: application/symlink" -H "X-Symlink-Target: swift-http-import-source/${DISAMBIGUATOR}/just/some/files/1.txt" "${SOURCE_URL}/just/a/symlink.txt"
sleep "$SLEEP" # wait for container listing to get updated

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${SOURCE_SPEC}
      only: '/$|symlink\\.txt'
      to:
        container: ${CONTAINER_BASE}-test9
        object_prefix: only-symlink
    - from: ${SOURCE_SPEC}
      only: '/$|symlink|[12]\\.txt'
      to:
        container: ${CONTAINER_BASE}-test9
        object_prefix: symlink-and-target
EOF

expect test9 <<-EOF
>> only-symlink/just/a/symlink.txt
Hello World.
>> symlink-and-target/just/a/symlink.txt
Hello World.
>> symlink-and-target/just/some/files/1.txt
Hello World.
>> symlink-and-target/just/some/files/2.txt
Hello Second World.
EOF

# check that the "only-symlink" job transfers symlink.txt as a regular file (it cannot
# transfer as a symlink because the link target is missing on the target side)
if curl -si -H "X-Auth-Token: ${OS_AUTH_TOKEN}" "${OS_STORAGE_URL}/${CONTAINER_BASE}-test9/only-symlink/just/a/symlink.txt?symlink=get" | grep -qi '^X-Symlink-Target'; then
  printf "\e[1;31m>>\e[0;31m Expected only-symlink/just/a/symlink.txt not to be a symlink, but it is one:\e[0m\n"
  curl -si -H "X-Auth-Token: ${OS_AUTH_TOKEN}" "${OS_STORAGE_URL}/${CONTAINER_BASE}-test9/only-symlink/just/a/symlink.txt?symlink=get"
  exit 1
fi

# check that the "symlink-and-target" job transfers symlink.txt as a symlink
# (since its link target is also included in the job)
if ! curl -si -H "X-Auth-Token: ${OS_AUTH_TOKEN}" "${OS_STORAGE_URL}/${CONTAINER_BASE}-test9/symlink-and-target/just/a/symlink.txt?symlink=get" | grep -qi '^X-Symlink-Target'; then
  printf "\e[1;31m>>\e[0;31m Expected symlink-and-target/just/a/symlink.txt to be a symlink, but it is not:\e[0m\n"
  curl -si -H "X-Auth-Token: ${OS_AUTH_TOKEN}" "${OS_STORAGE_URL}/${CONTAINER_BASE}-test9/symlink-and-target/just/a/symlink.txt?symlink=get"
  exit 1
fi

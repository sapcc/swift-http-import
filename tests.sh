#!/bin/bash
set -euo pipefail
cd "$(readlink -f "$(dirname "$0")")"

if [ -z "${OS_AUTH_URL:-}" ]; then
  echo "!! This testcase needs OpenStack credentials in the usual OS_* variables." >&2
  exit 1
fi

# containe rnames
DISAMBIGUATOR="$(date +%s)"
CONTAINER_PUBLIC="swift-http-import-source"
CONTAINER_BASE="swift-http-import-${DISAMBIGUATOR}"
# a temporary file that is used for various purposes
TEST_FILENAME="$(basename "$(mktemp -p . tmp.XXXXXX)")"
# YAML object (except for {}) with the auth parameters from the environment
AUTH_PARAMS="
  auth_url:            \"${OS_AUTH_URL}\",
  user_name:           \"${OS_USERNAME}\",
  user_domain_name:    \"${OS_USER_DOMAIN_NAME}\",
  project_name:        \"${OS_PROJECT_NAME}\",
  project_domain_name: \"${OS_PROJECT_DOMAIN_NAME}\",
  password:            \"${OS_PASSWORD}\",
  region_name:         \"${OS_REGION_NAME:-}\",
"

# speed up swiftclient
export OS_AUTH_TOKEN="$(openstack token issue -f value -c id)"

################################################################################
# cleanup from previous test runs

step() {
  echo -e "\e[1;36m>>\e[0;36m $@...\e[0m"
}

cleanup_containers() {
  for CONTAINER_NAME in $(swift list | grep "^swift-http-import"); do
    step "Cleaning up container ${CONTAINER_NAME}"
    if [ "${CONTAINER_NAME}" = "${CONTAINER_PUBLIC}" ]; then
      # do not delete the public container itself; want to keep the metadata
      swift list "${CONTAINER_NAME}" | xargs -r swift delete "${CONTAINER_NAME}"
    else
      swift delete "${CONTAINER_NAME}"
    fi
  done
}

cleanup_containers

################################################################################
step 'Preparing source container'

upload_file_from_stdin() {
  # `swift upload` is stupid; it will stubbornly refuse any pipes or FIFOs and
  # only accept plain regular files, so I have to use a temp file here
  sed 's/^  //' > "${TEST_FILENAME}"
  swift upload "${CONTAINER_PUBLIC}" "${TEST_FILENAME}" --object-name "${DISAMBIGUATOR}/$1"
}

upload_file_from_stdin just/some/files/1.txt <<-EOF
  Hello World.
EOF
upload_file_from_stdin just/some/files/2.txt <<-EOF
  Hello Second World.
EOF

swift post "${CONTAINER_PUBLIC}" -r '.r:*,.rlistings' -m 'web-listings: true'
sleep 15 # wait for container listing to get updated

# get public HTTP URL for container
CONTAINER_PUBLIC_URL="$(swift stat -v "${CONTAINER_PUBLIC}" | awk '$1=="URL:"{print$2}')/${DISAMBIGUATOR}"

################################################################################
# functions for tests

mirror() {
  # config file comes from stdin
  ./swift-http-import /proc/self/fd/0
  # wait for container listing to get updated
  sleep 15
}

dump() {
  local CONTAINER="${CONTAINER_BASE}-$1"
  local FILENAME
  swift list "${CONTAINER}" | while read FILENAME; do
    echo ">> ${FILENAME}"
    swift download -o - "${CONTAINER}" "${FILENAME}"
  done || true
}

expect() {
  local ACTUAL="$(dump "$1")"
  local EXPECTED="$(cat)"
  if ! diff -q <(echo "${EXPECTED}") <(echo "${ACTUAL}") > /dev/null; then
    echo -e "\e[1;31m>>\e[0;31m Contents of target container ${CONTAINER_BASE}-$1 do not match expectation. Diff follows:\e[0m"
    diff -u <(echo "${EXPECTED}") <(echo "${ACTUAL}")
  fi
}

################################################################################
step 'Test 1: Mirror from HTTP'

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${CONTAINER_PUBLIC_URL}
      to: ${CONTAINER_BASE}-test1
EOF

expect test1 <<-EOF
>> just/some/files/1.txt
Hello World.
>> just/some/files/2.txt
Hello Second World.
EOF

################################################################################
step 'Test 1 (cont.): Add another file and sync again'

upload_file_from_stdin just/another/file.txt <<-EOF
  Hello Another World.
EOF
sleep 15 # wait for container listing to get updated

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${CONTAINER_PUBLIC_URL}
      to: ${CONTAINER_BASE}-test1
EOF

expect test1 <<-EOF
>> just/another/file.txt
Hello Another World.
>> just/some/files/1.txt
Hello World.
>> just/some/files/2.txt
Hello Second World.
EOF

################################################################################
step 'Test 2: Exclusion regex'

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${CONTAINER_PUBLIC_URL}
      to: ${CONTAINER_BASE}-test2
      except: 'some/'
EOF

expect test2 <<-EOF
>> just/another/file.txt
Hello Another World.
EOF

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${CONTAINER_PUBLIC_URL}
      to: ${CONTAINER_BASE}-test2
      except: '2'
EOF

expect test2 <<-EOF
>> just/another/file.txt
Hello Another World.
>> just/some/files/1.txt
Hello World.
EOF

################################################################################
step 'Test 3: Inclusion regex'

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${CONTAINER_PUBLIC_URL}
      to: ${CONTAINER_BASE}-test3
      only: '[0-9].txt'
EOF

expect test3 </dev/null # empty because the inclusion regex did not match the directories along the path

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${CONTAINER_PUBLIC_URL}
      to: ${CONTAINER_BASE}-test3
      only: '/$|[0-9].txt'
EOF

expect test3 <<-EOF
>> just/some/files/1.txt
Hello World.
>> just/some/files/2.txt
Hello Second World.
EOF

################################################################################
step 'Test 4: Exclusion takes precedence over inclusion'

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${CONTAINER_PUBLIC_URL}
      to: ${CONTAINER_BASE}-test4
      only: '/$|[0-9].txt'
      except: '2'
EOF

expect test4 <<-EOF
>> just/some/files/1.txt
Hello World.
EOF

################################################################################
step 'Test 5: Immutability regex blocks re-transfer'

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${CONTAINER_PUBLIC_URL}
      to: ${CONTAINER_BASE}-test5
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
    - from: ${CONTAINER_PUBLIC_URL}
      to: ${CONTAINER_BASE}-test5
      only: '/$|file.txt'
      immutable: '.*.txt'
EOF

expect test5 <<-EOF
>> just/another/file.txt
Hello Another World.
EOF

################################################################################
# cleanup before exiting

cleanup_containers
rm -f "${TEST_FILENAME}"

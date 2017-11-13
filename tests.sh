#!/bin/bash
set -euo pipefail
cd "$(readlink -f "$(dirname "$0")")"

if [ $# -ne 1 ]; then
  echo "usage: ./tests.sh (http|swift)" >&2
  exit 1
fi
if [ "$1" != swift -a "$1" != http ]; then
  echo "usage: ./tests.sh (http|swift)" >&2
  exit 1
fi
if [ -z "${OS_AUTH_URL:-}" ]; then
  echo "!! This testcase needs OpenStack credentials in the usual OS_* variables." >&2
  exit 1
fi

# containe rnames
DISAMBIGUATOR="$(date +%s)"
CONTAINER_PUBLIC="swift-http-import-source"
CONTAINER_BASE="swift-http-import-${DISAMBIGUATOR}"
# a temporary file that is used for various purposes
TEST_FILENAME="$(basename "$(mktemp -p ${TMPDIR:-/tmp} tmp.XXXXXX)")"
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
eval "$(swift auth)"

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
  OBJECT_NAME="$1"
  shift
  swift upload "${CONTAINER_PUBLIC}" "${TEST_FILENAME}" --object-name "${DISAMBIGUATOR}/${OBJECT_NAME}" "$@"
}

upload_file_from_stdin just/some/files/1.txt <<-EOF
  Hello World.
EOF
upload_file_from_stdin just/some/files/2.txt <<-EOF
  Hello Second World.
EOF

swift post "${CONTAINER_PUBLIC}" -r '.r:*,.rlistings' -m 'web-listings: true'
sleep 15 # wait for container listing to get updated

if [ "$1" = swift ]; then
  SOURCE_SPEC="{ container: \"${CONTAINER_PUBLIC}\", object_prefix: \"${DISAMBIGUATOR}\", ${AUTH_PARAMS} }"
else
  # get public HTTP URL for container
  SOURCE_SPEC="{ url: \"$(swift stat -v "${CONTAINER_PUBLIC}" | awk '$1=="URL:"{print$2}')/${DISAMBIGUATOR}/\" }"
fi

################################################################################
# functions for tests

mirror() {
  # config file comes from stdin
  ./build/swift-http-import /dev/fd/0
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
step 'Test 1 (cont.): Add another file and sync again'

upload_file_from_stdin just/another/file.txt <<-EOF
  Hello Another World.
EOF
sleep 15 # wait for container listing to get updated

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

################################################################################
step 'Test 2: Exclusion regex'

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

################################################################################
step 'Test 3: Inclusion regex'

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

################################################################################
step 'Test 4: Exclusion takes precedence over inclusion'

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

################################################################################
step 'Test 5: Immutability regex blocks re-transfer'

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

################################################################################
step 'Test 6: Segmenting of large files'

upload_file_from_stdin largefile.txt <<-EOF
  Line number 1
  Line number 2
  Line number 3
  Line number 4
  Line number 5
EOF

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${SOURCE_SPEC}
      to: { container: ${CONTAINER_BASE}-test6 }
      segmenting:
        container: ${CONTAINER_BASE}-test6-segments
        min_bytes: 30
        segment_bytes: 14
EOF
# NOTE: A segment size of 14 bytes should put each line of text in its own
# segment, i.e. 5 segments.

expect test6 <<-EOF
>> just/another/file.txt
This is the new file!
>> just/some/files/1.txt
Hello World.
>> just/some/files/2.txt
Hello Second World.
>> largefile.txt
Line number 1
Line number 2
Line number 3
Line number 4
Line number 5
EOF

SEGMENT_COUNT="$(swift list ${CONTAINER_BASE}-test6-segments | wc -l)"
if [ "${SEGMENT_COUNT}" -ne 5 ]; then
  echo -e "\e[1;31m>>\e[0;31m Expected SLO to have 5 segments, but got ${SEGMENT_COUNT} instead:\e[0m"
  swift list ${CONTAINER_BASE}-test6-segments | sed 's/^/    /'
  exit 1
fi
# NOTE: This also ensures that the small files are not uploaded in segments,
# because then the segment count would be much more than 5.

################################################################################
step 'Test 7: Object expiration'

if [ "$1" = http ]; then
  echo ">> Test skipped (works only with Swift source)."
else

upload_file_from_stdin expires.txt -H 'X-Delete-At: 2000000000' <<-EOF
  This will expire soon.
EOF

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${SOURCE_SPEC}
      to: { container: ${CONTAINER_BASE}-test7 }
      only: 'expires.txt'
      expiration:
        delay_seconds: 42
EOF

expect test7 <<-EOF
>> expires.txt
This will expire soon.
EOF

EXPIRY_TIMESTAMP="$(swift stat ${CONTAINER_BASE}-test7 expires.txt | awk '/X-Delete-At:/ { print $2 }')"
if [ "${EXPIRY_TIMESTAMP}" != 2000000042 ]; then
  echo -e "\e[1;31m>>\e[0;31m Expected file to expire at timestamp 2000000042, but expires at timestamp '${EXPIRY_TIMESTAMP}' instead.\e[0m"
  exit 1
fi

fi # end of: if [ "$1" = http ]

################################################################################
# cleanup before exiting

cleanup_containers
rm -f "${TEST_FILENAME}"

#!/usr/bin/env bash
set -euo pipefail

if [[ ! -v LIB_SOURCED ]]; then
  export SOURCE_TYPE=swift
  cd "$(readlink -f "$(dirname "$0")")/.."
  # shellcheck disable=SC1090,SC1091
  source lib.sh
  setup "$@"
fi

step 'Test 20: Object expiration'

upload_file_from_stdin expires.txt -H 'X-Delete-At: 2000000000' <<-EOF
  This will expire soon.
EOF
upload_file_from_stdin expires-with-segments.txt -H 'X-Delete-At: 2000000000' <<-EOF
  This will expire soon.
  This will expire soon.
EOF

mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: ${SOURCE_SPEC}
      to: { container: ${CONTAINER_BASE}-test20 }
      only: 'expires.*txt'
      expiration:
        delay_seconds: 42
      segmenting:
        container: ${CONTAINER_BASE}-test20-segments
        min_bytes: 30
        segment_bytes: 30
EOF

expect test20 <<-EOF
>> expires-with-segments.txt
This will expire soon.
This will expire soon.
>> expires.txt
This will expire soon.
EOF

for OBJECT_NAME in expires.txt expires-with-segments.txt; do
  EXPIRY_TIMESTAMP="$(swift stat "${CONTAINER_BASE}-test20" "${OBJECT_NAME}" | awk '/X-Delete-At:/ { print $2 }')"
  if [ "${EXPIRY_TIMESTAMP}" != 2000000042 ]; then
    printf "\e[1;31m>>\e[0;31m Expected file \"%s\" to expire at timestamp 2000000042, but expires at timestamp '%s' instead.\e[0m\n" "$OBJECT_NAME" "$EXPIRY_TIMESTAMP"
    exit 1
  fi
done

# also check that expiration dates are applied to the segments as well
swift list "${CONTAINER_BASE}-test20-segments" | while read -r OBJECT_NAME; do
  EXPIRY_TIMESTAMP="$(swift stat "${CONTAINER_BASE}-test20-segments" "${OBJECT_NAME}" | awk '/X-Delete-At:/ { print $2 }')"
  if [ "${EXPIRY_TIMESTAMP}" != 2000000042 ]; then
    printf "\e[1;31m>>\e[0;31m Expected segment '%s' to expire at timestamp 2000000042, but expires at timestamp '%s' instead.\e[0m\n" "$OBJECT_NAME" "$EXPIRY_TIMESTAMP"
    exit 1
  fi
done || (
  printf "\e[1;31m>>\e[0;31m Expected object 'expires-with-segments.txt' to be an SLO, but it's not segmented.\e[0m\n"
  exit 1
)

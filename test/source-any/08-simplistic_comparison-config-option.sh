#!/usr/bin/env bash
set -euo pipefail

if [[ ! -v LIB_SOURCED ]]; then
  cd "$(readlink -f "$(dirname "$0")")/.."
  # shellcheck disable=SC1090,SC1091
  source lib.sh
  setup "$@"
fi

step 'Test 08: "simplistic_comparison" config option'

if ! hash rclone &>/dev/null; then
  echo ">> Test skipped (rclone is not installed, instructions: https://rclone.org/install/)."
else

  rclone_cmd() {
    RCLONE_CONFIG_TESTREMOTE_TYPE=swift RCLONE_CONFIG_TESTREMOTE_ENV_AUTH=1 rclone "$@"
  }

  upload_test_file_using_rclone() {
    local file_name="$1"
    echo "This is a test file." >"${file_name}"
    rclone_cmd copy "${file_name}" TESTREMOTE:"${CONTAINER_BASE}/from"
    rclone_cmd copy "${file_name}" TESTREMOTE:"${CONTAINER_BASE}/to"
  }

  if hash gdate &>/dev/null; then
    date() { gdate "$@"; }
  fi

  get_swift_object_mtime() {
    date -d "$(swift stat "${CONTAINER_BASE}" "$1" \
      | grep 'Last Modified:' \
      | sed -E 's/Last Modified:\s*(.*)/\1/')" '+%s'
  }

  # upload test files using rclone and get their mtime
  upload_test_file_using_rclone "${TEST_DIR}/rclone-test-file-1"
  upload_test_file_using_rclone "${TEST_DIR}/rclone-test-file-2"
  sleep "$SLEEP" # wait for container listing to get updated

  before_mtime_1="$(get_swift_object_mtime to/rclone-test-file-1)"
  before_mtime_2="$(get_swift_object_mtime to/rclone-test-file-2)"

  # mirror test files using swift-http-import and compare the mtime
  mirror <<-EOF
  swift: { $AUTH_PARAMS }
  jobs:
    - from: { container: ${CONTAINER_BASE}, object_prefix: from, ${AUTH_PARAMS} }
      to:
        container: ${CONTAINER_BASE}
        object_prefix: to
      match:
        simplistic_comparison: true
EOF

  after_mtime_1="$(get_swift_object_mtime to/rclone-test-file-1)"
  after_mtime_2="$(get_swift_object_mtime to/rclone-test-file-2)"

  if ! [[ "$before_mtime_1" == "$after_mtime_1" || ! "$before_mtime_2" == "$after_mtime_2" ]]; then
    printf "\e[1;31m>>\e[0;31m Files in %s have been modified by swift-http-import. They were expected not to be modified.\e[0m\n" "$CONTAINER_BASE"
    exit 1
  fi

fi # end of: if ! hash rclone &>/dev/null

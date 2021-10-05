#!/usr/bin/env bash

# exit early if we already sourced the lib
[[ ${LIB_SOURCED:-} == 1 ]] && return

SLEEP=${SLEEP:-1}
# container names
DISAMBIGUATOR="$(date +%s)-$RANDOM"
CONTAINER_PUBLIC="swift-http-import-source-${DISAMBIGUATOR}"
CONTAINER_BASE="swift-http-import-${DISAMBIGUATOR}"
# a temporary file that is used for various purposes
TEST_DIR="$(mktemp -d)"
TEST_FILENAME="$(mktemp "${TEST_DIR}/tmp.XXXXXX")"
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
  printf "\e[1;36m>>\e[0;36m %s...\e[0m\n" "$@"
}

cleanup_containers() {
  for CONTAINER_NAME in $(swift list | grep "^swift-http-import"); do
    step "Cleaning up container ${CONTAINER_NAME}"
    if [[ "${CONTAINER_NAME}" == "${CONTAINER_PUBLIC}" ]]; then
      # macOS's xargs does not support -r
      if [[ "$(uname -s)" != "Darwin" ]]; then
        xargs() { command xargs -r "$@"; }
      fi
      # do not delete the public container itself; want to keep the metadata
      swift list "${CONTAINER_NAME}" | xargs swift delete "${CONTAINER_NAME}"
    else
      swift delete "${CONTAINER_NAME}"
    fi
  done
}

# cleanup when exiting the script early
trap 'rm -rf $TEST_DIR' EXIT INT TERM

################################################################################

upload_file_from_stdin() {
  # `swift upload` is stupid; it will stubbornly refuse any pipes or FIFOs and
  # only accept plain regular files, so I have to use a temp file here
  sed 's/^  //' >"${TEST_FILENAME}"
  OBJECT_NAME="$1"
  shift
  swift upload "${CONTAINER_PUBLIC}" "${TEST_FILENAME}" --object-name "${DISAMBIGUATOR}/${OBJECT_NAME}" "$@"
}

setup() {
  step 'Preparing source container'

  swift post "${CONTAINER_PUBLIC}" -r '.r:*,.rlistings' -m 'web-listings: true'
  sleep "$SLEEP" # wait for container listing to get updated

  # get public HTTP URL for container
  SOURCE_URL="$(swift stat -v "${CONTAINER_PUBLIC}" | awk '$1 == "URL:"{ print $2 }')/${DISAMBIGUATOR}"

  # if SOURCE_TYPE is unset try loading it from the $1
  SOURCE_TYPE=${SOURCE_TYPE:-$1}
  if [[ ${SOURCE_TYPE:-} == swift ]]; then
    export SOURCE_SPEC="{ container: \"${CONTAINER_PUBLIC}\", object_prefix: \"${DISAMBIGUATOR}\", ${AUTH_PARAMS} }"
  elif [[ ${SOURCE_TYPE:-} == http ]]; then
    export SOURCE_SPEC="{ url: \"${SOURCE_URL}/\" }"
  else
    echo "\$SOURCE_TYPE needs to be set to either \`http\` or \`swift\`. You can either export the variable or supply the value as the first argument."
    exit 1
  fi
}

################################################################################
# functions for tests

upload_target_file_from_stdin() {
  # same trickery as in `upload_file_from_stdin` (see comments over there), but
  # for a target container
  sed 's/^  //' >"${TEST_FILENAME}"
  local CONTAINER_NAME="${CONTAINER_BASE}-$1"
  local OBJECT_NAME="$2"
  shift 2
  swift upload "${CONTAINER_NAME}" "${TEST_FILENAME}" --object-name "${OBJECT_NAME}" "$@"
}

mirror() {
  # config file comes from stdin
  ../build/swift-http-import /dev/fd/0
  # wait for container listing to get updated
  sleep "$SLEEP"
}

dump() {
  local CONTAINER="${CONTAINER_BASE}-$1"
  local FILENAME
  swift list "${CONTAINER}" | while read -r FILENAME; do
    echo ">> ${FILENAME}"
    swift download -o - "${CONTAINER}" "${FILENAME}"
  done || true
}

expect() {
  local ACTUAL EXPECTED

  ACTUAL="$(dump "$1")"
  EXPECTED="$(cat)"
  if ! diff -q <(echo "${EXPECTED}") <(echo "${ACTUAL}") >/dev/null; then
    printf "\e[1;31m>>\e[0;31m Contents of target container %s-%s do not match expectation. Diff follows:\e[0m\n" "$CONTAINER_BASE" "$1"
    diff -u <(echo "${EXPECTED}") <(echo "${ACTUAL}")
  fi
}

# this should needs to be last
LIB_SOURCED=1

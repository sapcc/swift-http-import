#!/usr/bin/env bash

# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

cd "$(readlink -f "$(dirname "$0")")"

if [[ $# -ne 1 ]]; then
  echo -e "usage: ./$0 [all|http|swift]" >&2
  exit 1
fi

if [[ ! -v OS_AUTH_URL ]]; then
  echo "!! This testcase needs OpenStack credentials in the usual OS_* variables." >&2
  exit 1
fi

# shellcheck source=lib.sh disable=SC1091
source lib.sh

# cleanup when exiting the script early
trap 'for pid in "${pids[@]}"; do kill "$pid" &>/dev/null || true; done; cleanup_containers' EXIT

start_test() {
  # NOTE: running tests in parallel appears to be unstable, so do not do it by default
  if [[ ${PARALLEL:-false} = true ]]; then
    bash -c "export SOURCE_TYPE=$SOURCE_TYPE && source lib.sh && setup && source $1" &
    pids+=($!)
  else
    bash -c "export SOURCE_TYPE=$SOURCE_TYPE && source lib.sh && setup && source $1"
  fi
}

case "$1" in
all)
  for source in http swift; do
    if [[ ${PARALLEL:-false} = true ]]; then
      ./run.sh "$source" &
      pids+=($!)
    else
      ./run.sh "$source"
    fi
  done
  ;;
http|swift)
  export SOURCE_TYPE=$1
  cleanup_containers
  setup
  for file in source-{any,$1}/*.sh; do
    start_test "$file"
  done
  ;;
*)
  echo -e "\n\nusage: ./$0 [all|http|swift]\n" >&2
  exit 1
  ;;
esac

fail=0
for pid in "${pids[@]}"; do
  if ! wait "$pid"; then
    echo "A test failed. See above for details."
    fail=1
  fi
done

exit "$fail"

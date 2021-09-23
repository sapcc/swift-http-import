#!/usr/bin/env bash
# shellcheck disable=SC1090,SC1091
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

source lib.sh

case "$1" in
all)
  for file in source-*/*.sh; do
    source "$file"
  done
  ;;
http)
  for file in source-{any,http}/*.sh; do
    source "$file"
  done
  ;;
swift)
  for file in source-{any,http}/*.sh; do
    source "$file"
  done
  ;;
*)
  echo -e "\n\nusage: ./$0 [all|http|swift]\n" >&2
  exit 1
  ;;
esac

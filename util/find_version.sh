#!/bin/sh
awk '$1 == "#" && !/TBD/ { print $2 }' CHANGELOG.md | sed 's/^v//' | head -n1

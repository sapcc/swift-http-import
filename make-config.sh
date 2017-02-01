#!/bin/bash

# This helper script can be used to quickly generate an initial config.yaml for
# swift-http-import. AuthN parameters are read from the usual OS_* environment
# variables, and the source and target for the first sync job can be given as
# arguments.

if [ $# -ne 2 ]; then
    echo "Usage: $0 <from-url> <to-container-and-path>" >&2
    exit 1
fi

sed '/region_name:\s*$/d' <<-EOF
swift:
  auth_url:            ${OS_AUTH_URL}
  user_name:           ${OS_USERNAME}
  user_domain_name:    ${OS_USER_DOMAIN_NAME}
  project_name:        ${OS_PROJECT_NAME}
  project_domain_name: ${OS_PROJECT_DOMAIN_NAME}
  password:            ${OS_PASSWORD}
  region_name:         ${OS_REGION_NAME}

workers:
  transfer: 1

jobs:
  - from: $1
    to:   $2
EOF

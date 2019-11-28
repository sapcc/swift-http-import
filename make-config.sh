#!/bin/bash

# This helper script can be used to quickly generate an initial config.yaml for
# swift-http-import. AuthN parameters are read from the usual OS_* environment
# variables, and the source and target for the first sync job can be given as
# arguments.

if [ $# -lt 2 -o $# -gt 3 ]; then
    echo "Usage: $0 <from-url> <to-container> [<object-prefix>]" >&2
    exit 1
fi
if [[ "$2" == */* ]]; then # container name may not contain slash
    echo "Usage: $0 <from-url> <to-container> [<object-prefix>]" >&2
    exit 1
fi

sed '/\(region_name\|object_prefix\):\s*$/d' <<-EOF
swift:
  auth_url:                      ${OS_AUTH_URL}
  user_name:                     ${OS_USERNAME}
  user_domain_name:              ${OS_USER_DOMAIN_NAME}
  project_name:                  ${OS_PROJECT_NAME}
  project_domain_name:           ${OS_PROJECT_DOMAIN_NAME}
  password:                      ${OS_PASSWORD}
  region_name:                   ${OS_REGION_NAME}
  application_credential_id:     ${OS_APPLICATION_CREDENTIAL_ID}
  application_credential_name:   ${OS_APPLICATION_CREDENTIAL_NAME}
  application_credential_secret: ${OS_APPLICATION_CREDENTIAL_SECRET}

workers:
  transfer: 1

jobs:
  - from:
      url: $1
    to:
     container: $2
     object_prefix: $3
EOF

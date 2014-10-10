#!/bin/bash

# This script builds all images locally (requires Docker)

set -o errexit
set -o nounset
set -o pipefail

hackdir=$(CDPATH="" cd $(dirname $0); pwd)

# Set the environment variables required by the build.
. "${hackdir}/config-go.sh"

# Go to the top of the tree.
cd "${OS_REPO_ROOT}"

docker build -t sti_test/sti-fake test/integration/images/sti-fake
docker build -t sti_test/sti-fake-broken test/integration/images/sti-fake-broken 
docker build -t sti_test/sti-fake-user test/integration/images/sti-fake-user

#!/bin/bash

# This script builds all images locally (requires Docker)

set -o errexit
set -o nounset
set -o pipefail

STI_ROOT=$(dirname "${BASH_SOURCE}")/..

# Go to the top of the tree.
cd "${STI_ROOT}"

docker build -t sti_test/sti-fake test/integration/images/sti-fake
docker build -t sti_test/sti-fake-broken test/integration/images/sti-fake-broken
docker build -t sti_test/sti-fake-user test/integration/images/sti-fake-user
docker build -t sti_test/sti-fake-with-scripts test/integration/images/sti-fake-with-scripts

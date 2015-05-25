#!/bin/bash

# This script builds all images locally (requires Docker)

set -o errexit
set -o nounset
set -o pipefail

STI_ROOT=$(dirname "${BASH_SOURCE}")/..

# Go to the top of the tree.
cd "${STI_ROOT}"


function buildimage()
{
    tag=$1
    src=$2
    cp -R test/integration/scripts $src
    docker build -t "${tag}" "${src}"
    rm -rf $src/scripts
}

buildimage sti_test/sti-fake test/integration/images/sti-fake
buildimage sti_test/sti-fake-env test/integration/images/sti-fake-env
buildimage sti_test/sti-fake-user test/integration/images/sti-fake-user
buildimage sti_test/sti-fake-scripts test/integration/images/sti-fake-scripts
buildimage sti_test/sti-fake-scripts-no-save-artifacts test/integration/images/sti-fake-scripts-no-save-artifacts
buildimage sti_test/sti-fake-no-tar test/integration/images/sti-fake-no-tar

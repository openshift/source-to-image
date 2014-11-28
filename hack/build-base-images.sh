#!/bin/bash

# This script builds release images for use by the release build.
#
# Set STI_IMAGE_PUSH=true to push images to a registry
#

set -o errexit
set -o nounset
set -o pipefail

STI_ROOT=$(dirname "${BASH_SOURCE}")/..
source "${STI_ROOT}/hack/common.sh"

# Go to the top of the tree.
cd "${STI_ROOT}"

# Build the images
docker build --tag openshift/sti-release "${STI_ROOT}/images/release"

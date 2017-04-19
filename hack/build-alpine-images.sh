#!/bin/bash

# This script builds an alpine based s2i image for pushing to Dockerhub
#
# Set S2I_IMAGE_PUSH=true to push images to a registry
#

set -o errexit
set -o nounset
set -o pipefail

S2I_ROOT=$(dirname "${BASH_SOURCE}")/..
source "${S2I_ROOT}/hack/common.sh"

# Go to the top of the tree.
cd "${S2I_ROOT}"

s2i::build::get_version_vars

# Build the images
docker build --tag openshift/s2i:${S2I_GIT_VERSION} -f "${S2I_ROOT}/images/alpine/Dockerfile" .

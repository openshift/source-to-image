#!/bin/bash

# This script builds release images for use by the release build.
#
# Set S2I_IMAGE_PUSH=true to push images to a registry
#

set -o errexit
set -o nounset
set -o pipefail

S2I_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
source "${S2I_ROOT}/hack/common.sh"

s2i::build::get_version_vars
s2i::build::save_version_vars "${S2I_ROOT}/sti-version-defs"

# Go to the top of the tree.
pushd "${S2I_ROOT}"

# Build the images
docker build --tag "openshift/sti-release:${S2I_GIT_COMMIT}" "${S2I_ROOT}" -f "${S2I_ROOT}/images/release/Dockerfile"

popd
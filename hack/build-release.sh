#!/bin/bash

# This script generates release zips into _output/releases. It requires the openshift/sti-release
# image to be built prior to executing this command.

set -o errexit
set -o nounset
set -o pipefail
set -e

STARTTIME=$(date +%s)
S2I_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
source "${S2I_ROOT}/hack/common.sh"
source "${S2I_ROOT}/hack/util.sh"
s2i::log::install_errexit

buildCmd=${S2I_BUILD_CMD:-"podman"}

# Go to the top of the tree.
cd "${S2I_ROOT}"

# Build the images
echo "++ Building openshift/sti-release"
$buildCmd build -q --tag openshift/sti-release "${S2I_ROOT}/images/release"

# Clean existing output.
rm -rf "${S2I_LOCAL_RELEASEPATH}"
mkdir -p "${S2I_LOCAL_RELEASEPATH}"

# Generate version definitions.
# You can commit a specific version by specifying S2I_GIT_COMMIT="" prior to build
s2i::build::get_version_vars
s2i::build::save_version_vars "${S2I_ROOT}/sti-version-defs"

echo "++ Building release ${S2I_GIT_VERSION}"

# Perform the build and release in podman or docker.
if [[ "$(go env GOHOSTOS)" == "darwin" ]]; then
    $buildCmd run --rm -it -e RELEASE_LDFLAGS="-w -s" \
  -v "${S2I_ROOT}":/opt/app-root/src/source-to-image \
  openshift/sti-release
  else
    $buildCmd run --rm -it -e RELEASE_LDFLAGS="-w -s" \
  -v "${S2I_ROOT}":/opt/app-root/src/source-to-image:z \
  openshift/sti-release
  fi

echo "${S2I_GIT_COMMIT}" > "${S2I_LOCAL_RELEASEPATH}/.commit"

ret=$?; ENDTIME=$(date +%s); echo "$0 took $((ENDTIME - STARTTIME)) seconds"; exit "$ret"

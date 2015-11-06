#!/bin/bash

# This script generates release zips into _output/releases. It requires the openshift/sti-release
# image to be built prior to executing this command.

set -o errexit
set -o nounset
set -o pipefail

STARTTIME=$(date +%s)
STI_ROOT=$(dirname "${BASH_SOURCE}")/..
source "${STI_ROOT}/hack/common.sh"
source "${STI_ROOT}/hack/util.sh"
sti::log::install_errexit

# Go to the top of the tree.
cd "${STI_ROOT}"

# Build the images
echo "++ Building openshift/sti-release"
docker build -q --tag openshift/sti-release "${STI_ROOT}/images/release"

context="${STI_ROOT}/_output/buildenv-context"

# Clean existing output.
rm -rf "${STI_LOCAL_RELEASEPATH}"
rm -rf "${context}"
mkdir -p "${context}"
mkdir -p "${STI_OUTPUT}"

# Generate version definitions.
# You can commit a specific version by specifying STI_GIT_COMMIT="" prior to build
sti::build::get_version_vars
sti::build::save_version_vars "${context}/sti-version-defs"

echo "++ Building release ${STI_GIT_VERSION}"

# Create the input archive.
git archive --format=tar -o "${context}/archive.tar" "${STI_GIT_COMMIT}"
tar -rf "${context}/archive.tar" -C "${context}" sti-version-defs
gzip -f "${context}/archive.tar"

# Perform the build and release in Docker.
cat "${context}/archive.tar.gz" | docker run -i --cidfile="${context}/cid" openshift/sti-release
docker cp $(cat ${context}/cid):/go/src/github.com/openshift/source-to-image/_output/local/releases "${STI_OUTPUT}"
echo "${STI_GIT_COMMIT}" > "${STI_LOCAL_RELEASEPATH}/.commit"

ret=$?; ENDTIME=$(date +%s); echo "$0 took $(($ENDTIME - $STARTTIME)) seconds"; exit "$ret"

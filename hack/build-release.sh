#!/bin/bash

# This script generates release zips into _output/releases. It requires the openshift/sti-release
# image to be built prior to executing this command.

set -o errexit
set -o nounset
set -o pipefail

STI_ROOT=$(dirname "${BASH_SOURCE}")/..
source "${STI_ROOT}/hack/common.sh"

# Go to the top of the tree.
cd "${STI_ROOT}"

# Build the images
echo "++ Building openshift/sti-release"
docker build -q --tag openshift/sti-release "${STI_ROOT}/images/release"

context="${STI_ROOT}/_output/buildenv-context"

# Clean existing output.
rm -rf "${STI_ROOT}/_output/local/releases"
rm -rf "${STI_ROOT}/_output/local/go/bin"
rm -rf "${context}"
mkdir -p "${context}"
mkdir -p "${STI_ROOT}/_output/local"

# Generate version definitions.
sti::build::get_version_vars
sti::build::save_version_vars "${context}/sti-version-defs"

# Create the input archive.
git archive --format=tar -o "${context}/archive.tar" HEAD
tar -rf "${context}/archive.tar" -C "${context}" sti-version-defs
gzip -f "${context}/archive.tar"

# Perform the build and release in Docker.
cat "${context}/archive.tar.gz" | docker run -i --cidfile="${context}/cid" openshift/sti-release
docker cp $(cat ${context}/cid):/go/src/github.com/openshift/source-to-image/_output/local/releases "${STI_ROOT}/_output/local"
echo "${STI_GIT_COMMIT}" > "${STI_ROOT}/_output/local/releases/.commit"

# Copy the linux release archives release back to the local _output/local/go/bin directory.
sti::build::detect_local_release_tars "linux"

mkdir -p "${STI_LOCAL_BINPATH}"
tar mxzf "${STI_PRIMARY_RELEASE_TAR}" -C "${STI_LOCAL_BINPATH}"

sti::build::make_binary_symlinks

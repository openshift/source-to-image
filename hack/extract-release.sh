#!/bin/bash

# This script extracts a valid release tar into _output/releases. It requires hack/build-release.sh
# to have been executed

set -o errexit
set -o nounset
set -o pipefail

STI_ROOT=$(dirname "${BASH_SOURCE}")/..
source "${STI_ROOT}/hack/common.sh"

# Go to the top of the tree.
cd "${STI_ROOT}"

# Copy the linux release archives release back to the local _output/local/bin/linux/amd64 directory.
# TODO: support different OS's?
s2i::build::detect_local_release_tars "linux-amd64"

mkdir -p "${STI_OUTPUT_BINPATH}/linux/amd64"
tar mxzf "${STI_PRIMARY_RELEASE_TAR}" -C "${STI_OUTPUT_BINPATH}/linux/amd64"

s2i::build::make_binary_symlinks

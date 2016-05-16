#!/bin/bash

# Build all cross compile targets and the base binaries

set -o errexit
set -o nounset
set -o pipefail

STARTTIME=$(date +%s)
STI_ROOT=$(dirname "${BASH_SOURCE}")/..
source "${STI_ROOT}/hack/common.sh"
source "${STI_ROOT}/hack/util.sh"
s2i::log::install_errexit

# Build the primary for all platforms
STI_BUILD_PLATFORMS=("${STI_CROSS_COMPILE_PLATFORMS[@]}")
s2i::build::build_binaries "${STI_CROSS_COMPILE_TARGETS[@]}"

# Make the primary release.
STI_RELEASE_ARCHIVE="source-to-image"
STI_BUILD_PLATFORMS=("${STI_CROSS_COMPILE_PLATFORMS[@]}")
s2i::build::place_bins "${STI_CROSS_COMPILE_BINARIES[@]}"

ret=$?; ENDTIME=$(date +%s); echo "$0 took $(($ENDTIME - $STARTTIME)) seconds"; exit "$ret"

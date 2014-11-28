#!/bin/bash

# Build all cross compile targets and the base binaries

set -o errexit
set -o nounset
set -o pipefail

STI_ROOT=$(dirname "${BASH_SOURCE}")/..
source "${STI_ROOT}/hack/common.sh"

STI_BUILD_PLATFORMS=("${STI_COMPILE_PLATFORMS[@]-}")
sti::build::build_binaries "${STI_COMPILE_TARGETS[@]-}"

STI_BUILD_PLATFORMS=("${STI_CROSS_COMPILE_PLATFORMS[@]}")
sti::build::build_binaries "${STI_CROSS_COMPILE_TARGETS[@]}"

STI_RELEASE_ARCHIVES="${STI_OUTPUT}/releases"
sti::build::place_bins

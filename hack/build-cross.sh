#!/bin/bash

# Build all cross compile targets and the base binaries

set -o errexit
set -o nounset
set -o pipefail

STI_ROOT=$(dirname "${BASH_SOURCE}")/..
source "${STI_ROOT}/hack/common.sh"

# Build the primary for all platforms
STI_BUILD_PLATFORMS=("${STI_CROSS_COMPILE_PLATFORMS[@]}")
sti::build::build_binaries "${STI_CROSS_COMPILE_TARGETS[@]}"

# Build image binaries for a subset of platforms. Image binaries are currently
# linux-only, and are compiled with flags to make them static for use in Docker
# images "FROM scratch".
STI_BUILD_PLATFORMS=("${STI_IMAGE_COMPILE_PLATFORMS[@]-}")
CGO_ENABLED=0 STI_GOFLAGS="-a" sti::build::build_binaries "${STI_IMAGE_COMPILE_TARGETS[@]-}"

# Make the primary release.
STI_RELEASE_ARCHIVE="source-to-image"
STI_RELEASE_PLATFORMS=("${STI_CROSS_COMPILE_PLATFORMS[@]}")
STI_RELEASE_BINARIES=("${STI_CROSS_COMPILE_BINARIES[@]}")
sti::build::place_bins

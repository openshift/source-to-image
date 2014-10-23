#!/bin/bash

# This script sets up a go workspace locally and builds all go components.

set -o errexit
set -o nounset
set -o pipefail

hackdir=$(CDPATH="" cd $(dirname $0); pwd)

# Set the environment variables required by the build.
. "${hackdir}/config-go.sh"

# Go to the top of the tree.
cd "${STI_REPO_ROOT}"

if [[ $# == 0 ]]; then
  # Update $@ with the default list of targets to build.
  set -- cmd/sti
fi

binaries=()
for arg; do
  binaries+=("${STI_GO_PACKAGE}/${arg}")
done

build_tags=""
if [[ ! -z "$STI_BUILD_TAGS" ]]; then
  build_tags="-tags \"$STI_BUILD_TAGS\""
fi

go install $build_tags -ldflags "$(sti::build::ldflags)" "${binaries[@]}"

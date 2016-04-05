#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

STI_ROOT=$(dirname "${BASH_SOURCE}")/..

source "${STI_ROOT}/hack/common.sh"

sti::build::build_binaries "$@"

echo "+++ Updating Bash completion (${STI_ROOT}/contrib/bash/s2i)"
${STI_LOCAL_BINPATH}/s2i genbashcompletion > ${STI_ROOT}/contrib/bash/s2i

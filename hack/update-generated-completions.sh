#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

STI_ROOT=$(dirname "${BASH_SOURCE}")/..

source "${STI_ROOT}/hack/common.sh"

s2i::build::build_binaries "$@"

echo "+++ Updating Bash completion in contrib/bash/s2i"
${STI_LOCAL_BINPATH}/s2i genbashcompletion > ${STI_ROOT}/contrib/bash/s2i

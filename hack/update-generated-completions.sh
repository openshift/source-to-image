#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

S2I_ROOT=$(dirname "${BASH_SOURCE}")/..

source "${S2I_ROOT}/hack/common.sh"

s2i::build::build_binaries "$@"

echo "+++ Updating Bash completion in contrib/bash/s2i"
${S2I_LOCAL_BINPATH}/s2i genbashcompletion > ${S2I_ROOT}/contrib/bash/s2i

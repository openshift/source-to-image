#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

STI_ROOT=$(dirname "${BASH_SOURCE}")/../..
source "${STI_ROOT}/hack/buildenv/common.sh"

tar -C "${STI_RELEASES}" -cf - .

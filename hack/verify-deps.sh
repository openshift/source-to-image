#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

S2I_ROOT=$(dirname "${BASH_SOURCE}")/..
source "${S2I_ROOT}/hack/common.sh"

# double check the dependencies stored in vendor directory with the described dependency list in
# local Go Module files, go.mod and go.sum.
go mod verify
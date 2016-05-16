#!/bin/bash

set -e

S2I_ROOT=$(dirname "${BASH_SOURCE}")/..
source "${S2I_ROOT}/hack/common.sh"

GO_VERSION=($(go version))
echo "Detected go version: $(go version)"

go get golang.org/x/tools/cmd/cover github.com/tools/godep

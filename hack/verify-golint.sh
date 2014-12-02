#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

if ! which golint &>/dev/null; then
  echo "Unable to detect 'golint' package"
  echo "To install it, run: 'go get github.com/golang/lint/golint'"
  exit 1
fi

GO_VERSION=($(go version))
echo "Detected go version: $(go version)"

if [[ ${GO_VERSION[2]} != "go1.2" && ${GO_VERSION[2]} != "go1.3.1" && ${GO_VERSION[2]} != "go1.3.3" ]]; then
  echo "Unknown go version, skipping golint."
  exit 0
fi

STI_ROOT=$(dirname "${BASH_SOURCE}")/..
source "${STI_ROOT}/hack/common.sh"

cd "${STI_ROOT}"

find_files() {
  find . -not \( \
      \( \
        -wholename './output' \
        -o -wholename './_output' \
        -o -wholename './release' \
        -o -wholename './target' \
        -o -wholename '*/Godeps/*' \
      \) -prune \
    \) -name '*.go'
}

bad_files=$(find_files | xargs golint)
if [[ -n "${bad_files}" ]]; then
  echo "golint detected following problems:"
  echo "${bad_files}"
  exit 1
fi

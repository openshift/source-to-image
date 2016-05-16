#!/bin/bash

set -o nounset
set -o pipefail

GO_VERSION=($(go version))

if [[ -z $(echo "${GO_VERSION[2]}" | grep -E 'go1.[5-6]') ]]; then
  echo "Unknown go version '${GO_VERSION}', skipping go vet."
  exit 0
fi

S2I_ROOT=$(dirname "${BASH_SOURCE}")/..
source "${S2I_ROOT}/hack/common.sh"
source "${S2I_ROOT}/hack/util.sh"

cd "${S2I_ROOT}"
mkdir -p _output/govet

FAILURE=false
test_dirs=$(find_files | cut -d '/' -f 1-2 | sort -u)
for test_dir in $test_dirs
do
  go vet -shadow=false $test_dir
  if [ "$?" -ne 0 ]
  then
    FAILURE=true
  fi
done

# We don't want to exit on the first failure of go vet, so just keep track of
# whether a failure occurred or not.
if $FAILURE
then
  echo "FAILURE: go vet failed!"
  exit 1
else
  echo "SUCCESS: go vet succeded!"
  exit 0
fi

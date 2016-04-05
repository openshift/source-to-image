#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

STI_ROOT=$(dirname "${BASH_SOURCE}")/..
source "${STI_ROOT}/hack/common.sh"

cd "${STI_ROOT}"

mv contrib/bash/s2i contrib/bash/s2i-proposed
hack/update-generated-completions.sh

ret=0
diff -Naupr contrib/bash/s2i contrib/bash/s2i-proposed || ret=$?

mv contrib/bash/s2i-proposed contrib/bash/s2i
if [[ $ret -eq 0 ]]
then
  echo "SUCCESS: Generated completions up to date."
else
  echo "FAILURE: Generated completions out of date. Please run hack/update-generated-completions.sh"
  exit 1
fi

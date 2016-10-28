#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

S2I_ROOT=$(dirname "${BASH_SOURCE}")/..
source "${S2I_ROOT}/hack/common.sh"

cd "${S2I_ROOT}"

mv contrib/bash/s2i contrib/bash/s2i-proposed
trap "mv contrib/bash/s2i-proposed contrib/bash/s2i" exit
hack/update-generated-completions.sh

ret=0
diff -Naupr contrib/bash/s2i contrib/bash/s2i-proposed || ret=$?

if [[ $ret -eq 0 ]]
then
  echo "SUCCESS: Generated completions up to date."
else
  echo "FAILURE: Generated completions out of date. Please run hack/update-generated-completions.sh"
  exit 1
fi

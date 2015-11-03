#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

STI_ROOT=$(dirname "${BASH_SOURCE}")/..

function cleanup()
{
    set +e
    echo
    echo "Complete"
}

set +e
img_count=$(docker images | grep -c sti_test/sti-fake)
set -e

if [ "${img_count}" != "10" ]; then
    echo "You do not have necessary test images, be sure to run 'hack/build-test-images.sh' beforehand."
    exit 1
fi

trap cleanup EXIT SIGINT

export STI_TIMEOUT="-timeout 600s"
mkdir -p /tmp/sti
export LOG_FILE="$(mktemp -p /tmp/sti --suffix=integration.log)"

echo
echo Integration test cases ...
echo Log file: ${LOG_FILE}
echo

"${STI_ROOT}/hack/test-go.sh" test/integration -v -tags 'integration' "${@:1}" 2> ${LOG_FILE}

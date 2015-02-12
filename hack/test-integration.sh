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

if [ "${img_count}" != "5" ]; then
    echo "You do not have necessary test images, be sure to run 'hack/build-test-images.sh' beforehand."
    exit 1
fi

trap cleanup EXIT SIGINT

echo
echo Integration test cases ...
echo
export STI_TIMEOUT="-timeout 240s"
"${STI_ROOT}/hack/test-go.sh" test/integration -tags 'integration' "${@:1}"

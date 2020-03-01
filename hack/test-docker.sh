#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

readonly S2I_ROOT=$(dirname "${BASH_SOURCE}")/..

s2i::cleanup() {
  echo
  echo "Complete"
}

if [[ "${S2I_CONTAINER_MANAGER}" == "buildah" ]] ; then
  img_count="$(buildah images | grep -c sti_test/sti-fake || :)"
else
  img_count="$(docker images | grep -c sti_test/sti-fake || :)"
fi

readonly img_count

if [ "${img_count}" != "12" ]; then
  echo "Missing test images, run 'hack/build-test-images.sh' and try again."
  exit 1
fi

trap s2i::cleanup EXIT SIGINT

export S2I_BUILD_TAGS="integration"
export S2I_TIMEOUT="-timeout 600s"

echo
echo "Running buildah integration tests ..."
echo

"${S2I_ROOT}/hack/test-go.sh" test/integration/buildah -v -failfast "${@:1}"

echo
echo "Running docker integration tests ..."
echo

"${S2I_ROOT}/hack/test-go.sh" test/integration/docker -v -failfast "${@:1}"

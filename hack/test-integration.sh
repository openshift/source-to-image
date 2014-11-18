#!/bin/bash

img_count=$(docker images | grep sti_test/sti-fake | wc -l)
if [ "${img_count}" != "3" ]; then
  echo "You do not have necessary test images, be sure to run 'hack/build-images.sh' beforehand."
  exit 1
fi


set -e

function cleanup()
{
    set +e
    echo
    echo "Complete"
}

trap cleanup EXIT SIGINT

echo
echo Integration test cases ...
echo
$(dirname $0)/../hack/test-go.sh test/integration -tags 'integration'

#!/bin/bash

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
$(dirname $0)/../hack/test-go.sh test/integration -integration

#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

STARTTIME=$(date +%s)

echo $(go version)

go get github.com/tools/godep

go get -d github.com/golang/lint/golint
pushd $GOPATH/src/github.com/golang/lint/golint
git checkout c5fb716d6688a859aae56d26d3e6070808df29f7
popd
go install github.com/golang/lint/golint

ret=$?; ENDTIME=$(date +%s); echo "$0 took $(($ENDTIME - $STARTTIME)) seconds"; exit "$ret"

#!/bin/bash

set -e

source $(dirname $0)/config-go.sh

find_test_dirs() {
  cd src/${STI_GO_PACKAGE}
  find . -not \( \
      \( \
        -wholename './third_party' \
        -wholename './Godeps' \
        -o -wholename './release' \
        -o -wholename './target' \
        -o -wholename '*/third_party/*' \
        -o -wholename '*/Godeps/*' \
        -o -wholename '*/_output/*' \
      \) -prune \
    \) -name '*_test.go' -print0 | xargs -0n1 dirname | sort -u | xargs -n1 printf "${STI_GO_PACKAGE}/%s\n"
}

# there is currently a race in the coverage code in tip.  Remove this when it is fixed
# see https://code.google.com/p/go/issues/detail?id=8630 for details.
if [ "${TRAVIS_GO_VERSION}" == "tip" ]; then
  STI_COVER=""
else
  # -covermode=atomic becomes default with -race in Go >=1.3
  if [ -z ${STI_COVER+x} ]; then
    STI_COVER="-cover -covermode=atomic"
  fi
fi
STI_TIMEOUT=${STI_TIMEOUT:--timeout 30s}

if [ -z ${STI_RACE+x} ]; then
  STI_RACE="-race"
fi

cd "${STI_TARGET}"

if [ "$1" != "" ]; then
  if [ -n "${STI_COVER}" ]; then
    STI_COVER="${STI_COVER} -coverprofile=tmp.out"
  fi

  go test $STI_RACE $STI_TIMEOUT $STI_COVER "$STI_GO_PACKAGE/$1" "${@:2}"
  exit 0
fi

find_test_dirs | xargs go test $STI_RACE $STI_TIMEOUT $STI_COVER "${@:2}"

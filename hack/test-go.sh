#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

STI_ROOT=$(dirname "${BASH_SOURCE}")/..
source "${STI_ROOT}/hack/common.sh"

# Go to the top of the tree.
cd "${STI_ROOT}"

sti::build::setup_env

find_test_dirs() {
  cd "${STI_ROOT}"
  find . -not \( \
      \( \
        -wholename './Godeps' \
        -o -wholename './release' \
        -o -wholename './target' \
        -o -wholename '*/Godeps/*' \
        -o -wholename '*/_output/*' \
        -o -wholename './.git' \
      \) -prune \
    \) -name '*_test.go' -print0 | xargs -0n1 dirname | sort -u | xargs -n1 printf "${STI_GO_PACKAGE}/%s\n"
}

# -covermode=atomic becomes default with -race in Go >=1.3
if [ -z ${STI_COVER+x} ]; then
  STI_COVER=""
fi

OUTPUT_COVERAGE=${OUTPUT_COVERAGE:-""}

if [ -n "${OUTPUT_COVERAGE}" ]; then
  if [ -z ${STI_RACE+x} ]; then
    STI_RACE="-race"
  fi
  if [ -z "${STI_COVER}" ]; then
    STI_COVER="-cover -covermode=atomic"
  fi
fi

if [ -z ${STI_RACE+x} ]; then
  STI_RACE=""
fi

STI_TIMEOUT=${STI_TIMEOUT:--timeout 30s}

if [ "${1-}" != "" ]; then
  test_packages="$STI_GO_PACKAGE/$1"
else
  test_packages=`find_test_dirs`
fi

if [[ -n "${STI_COVER}" && -n "${OUTPUT_COVERAGE}" ]]; then
  # Iterate over packages to run coverage
  test_packages=( $test_packages )
  for test_package in "${test_packages[@]}"
  do
    mkdir -p "$OUTPUT_COVERAGE/$test_package"
    STI_COVER_PROFILE="-coverprofile=$OUTPUT_COVERAGE/$test_package/profile.out"

    go test $STI_RACE $STI_TIMEOUT $STI_COVER "$STI_COVER_PROFILE" "$test_package" "${@:2}"
  done

  echo 'mode: atomic' > ${OUTPUT_COVERAGE}/profiles.out
  find $OUTPUT_COVERAGE -name profile.out | xargs sed '/^mode: atomic$/d' >> ${OUTPUT_COVERAGE}/profiles.out
  go tool cover "-html=${OUTPUT_COVERAGE}/profiles.out" -o "${OUTPUT_COVERAGE}/coverage.html"

  rm -rf $OUTPUT_COVERAGE/$STI_GO_PACKAGE
else
  go test $STI_RACE $STI_TIMEOUT $STI_COVER "${@:2}" $test_packages
fi

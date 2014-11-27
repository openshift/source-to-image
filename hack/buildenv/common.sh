#!/bin/bash

STI_ROOT=$(dirname "${BASH_SOURCE}")/../..
cd "${STI_ROOT}"

source "${STI_ROOT}/hack/common.sh"

readonly STI_TARGET="${STI_ROOT}/_output/build"
readonly STI_GO_PACKAGE=github.com/openshift/source-to-image
readonly STI_RELEASES="${STI_ROOT}/_output/releases"

compile_targets=(
  cmd/sti
)

mkdir -p "${STI_TARGET}"

if [[ ! -f "/sti-build-image" ]]; then
  echo "WARNING: This script should be run in the os-build container image!" >&2
fi

if [[ -f "./sti-version-defs" ]]; then
  source "./sti-version-defs"
else
  echo "WARNING: No version information provided in build image"
  readonly STI_VERSION="${STI_VERSION:-unknown}"
  readonly STI_GITCOMMIT="${STI_GITCOMMIT:-unknown}"
fi


function os::build::make_binary() {
  local -r gopkg=$1
  local -r bin=${gopkg##*/}

  echo "+++ Building ${bin} for ${GOOS}/${GOARCH}"
  pushd "${STI_ROOT}" >/dev/null
  godep go build -ldflags "${STI_LD_FLAGS-}" -o "${ARCH_TARGET}/${bin}" "${gopkg}"
  popd >/dev/null
}

function os::build::make_binaries() {
  [[ $# -gt 0 ]] || {
    echo "!!! Internal error. os::build::make_binaries called with no targets."
  }

  local -a targets=("$@")
  local -a binaries=()
  local target
  for target in "${targets[@]}"; do
    binaries+=("${STI_GO_PACKAGE}/${target}")
  done

  ARCH_TARGET="${STI_TARGET}/${GOOS}/${GOARCH}"
  mkdir -p "${ARCH_TARGET}"

  local b
  for b in "${binaries[@]}"; do
    os::build::make_binary "$b"
  done

  mkdir -p "${STI_RELEASES}"
  readonly ARCHIVE_NAME="openshift-sti-${STI_VERSION}-${STI_GITCOMMIT}-${GOOS}-${GOARCH}.tar.gz"
  tar -czf "${STI_RELEASES}/${ARCHIVE_NAME}" -C "${ARCH_TARGET}" .
}

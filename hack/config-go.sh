#!/bin/bash

# This script sets up a go workspace locally and builds all go components.
# You can 'source' this file if you want to set up GOPATH in your local shell.

hackdir=$(CDPATH="" cd $(dirname $0); pwd)
source "${hackdir}/common.sh"

if [[ -z "$(which go)" ]]; then
  echo "Can't find 'go' in PATH, please fix and retry." >&2
  echo "See http://golang.org/doc/install for installation instructions." >&2
  exit 1
fi

# Travis continuous build uses a head go release that doesn't report
# a version number, so we skip this check on Travis.  Its unnecessary
# there anyway.
if [[ "${TRAVIS:-}" != "true" ]]; then
  GO_VERSION=($(go version))
  if [[ "${GO_VERSION[2]}" < "go1.2" ]]; then
    echo "Detected go version: ${GO_VERSION[*]}." >&2
    echo "STI requires go version 1.2 or greater." >&2
    echo "Please install Go version 1.2 or later" >&2
    exit 1
  fi
fi

STI_REPO_ROOT=$(dirname "${BASH_SOURCE:-$0}")/..
case "$(uname)" in
  Darwin)
    # Make the path absolute if it is not.
    if [[ "${STI_REPO_ROOT}" != /* ]]; then
      STI_REPO_ROOT=${PWD}/${STI_REPO_ROOT}
    fi
    ;;
  Linux)
    # Resolve symlinks.
    STI_REPO_ROOT=$(readlink -f "${STI_REPO_ROOT}")
    ;;
  *)
    echo "Unsupported operating system: \"$(uname)\"" >&2
    exit 1
esac

STI_TARGET="${STI_REPO_ROOT}/_output/go"
mkdir -p "${STI_TARGET}"

STI_GO_PACKAGE=github.com/openshift/source-to-image
STI_GO_PACKAGE_DIR="${STI_TARGET}/src/${STI_GO_PACKAGE}"

STI_GO_PACKAGE_BASEDIR=$(dirname "${STI_GO_PACKAGE_DIR}")
mkdir -p "${STI_GO_PACKAGE_BASEDIR}"

# Create symlink under _output/go/src.
ln -snf "${STI_REPO_ROOT}" "${STI_GO_PACKAGE_DIR}"

GOPATH="${STI_TARGET}:${STI_REPO_ROOT}/Godeps/_workspace"
export GOPATH

# Unset GOBIN in case it already exists in the current session.
unset GOBIN

STI_BUILD_TAGS=${STI_BUILD_TAGS-}

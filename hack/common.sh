#!/bin/bash

# This script provides common script functions for the hacks

# sti::build::gitcommit prints the current Git commit information
function sti::build::gitcommit() {
  set -o errexit
  set -o nounset
  set -o pipefail

  topdir=$(dirname "$0")/..
  cd "${topdir}"

  # TODO: when we start making tags, switch to git describe?
  if git_commit=$(git rev-parse --short "HEAD^{commit}" 2>/dev/null); then
    # Check if the tree is dirty.
    if ! dirty_tree=$(git status --porcelain) || [[ -n "${dirty_tree}" ]]; then
      echo "${git_commit}-dirty"
    else
      echo "${git_commit}"
    fi
  else
    echo "(none)"
  fi
  return 0
}


# sti::build::ldflags calculates the -ldflags argument for building STI
function sti::build::ldflags() {
  (
    # Run this in a subshell to prevent settings/variables from leaking.
    set -o errexit
    set -o nounset
    set -o pipefail

    topdir=$(dirname "$0")/..
    cd "${topdir}"

    declare -a ldflags=()
    ldflags+=(-X "github.com/openshift/source-to-image/pkg/sti/version.commitFromGit" "$(sti::build::gitcommit)")

    # The -ldflags parameter takes a single string, so join the output.
    echo "${ldflags[*]-}"
  )
}

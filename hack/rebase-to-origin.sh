#!/bin/bash
#
# This script will create a rebase commit in the OpenShift Origin Git repository
# based on the current HEAD.
#
# NOTE: Make sure all your changes are committed and there are no junk files
#       present in the pkg/ folder.
#

set -o errexit
set -o nounset
set -o pipefail

readonly S2I_ROOT=$(
  root=$(dirname "${BASH_SOURCE}")/..
  unset CDPATH
  cd "${root}"
  pwd
)
readonly OS_ROOT="${S2I_ROOT/%\/source-to-image/\/origin}"

source "${S2I_ROOT}/hack/util.sh"

# Exclude these packages from source-to-image explicitly
readonly exclude_pkgs=(
  pkg/cmd
  pkg/config
  pkg/create
  pkg/run
  pkg/test
  pkg/version
)

readonly origin_s2i_godep_dir="${OS_ROOT}/Godeps/_workspace/src/github.com/openshift/source-to-image"
readonly s2i_ref="$(git -C ${S2I_ROOT} rev-parse --verify HEAD)"
readonly s2i_short_ref="$(git -C ${S2I_ROOT} rev-parse --short HEAD)"
readonly s2i_godeps_ref="$(grep -m1 -A2 'openshift/source-to-image' ${OS_ROOT}/Godeps/Godeps.json |
  grep Rev | cut -d ':' -f2 | sed -e 's/"//g' -e 's/^[[:space:]]*//')"

pushd "${OS_ROOT}" >/dev/null
  git checkout -B "s2i-${s2i_short_ref}-bump" master
  rm -rf "${origin_s2i_godep_dir}"/*
  cp -R "${S2I_ROOT}/pkg" "${origin_s2i_godep_dir}/."

  # Remove all explicitly excluded packages
  for pkg in "${exclude_pkgs[@]}"; do
    rm -rvf "${origin_s2i_godep_dir}/${pkg}"
  done

  # Bump the origin Godeps.json file
  os::util::sed "s/${s2i_godeps_ref}/${s2i_ref}/g" "${OS_ROOT}/Godeps/Godeps.json"

  # Make a commit with proper message
  git add Godeps && git commit -m "bump(github.com/openshift/source-to-image): ${s2i_ref}"
popd

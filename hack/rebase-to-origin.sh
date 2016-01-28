#!/bin/bash -e
#
# This script will create a rebase commit in the OpenShift Origin GIT repository
# based on the current HEAD.
#
# NOTE: Make sure all your changes are committed and there are no junk files
#       present in the pkg/ folder.
#

source "${s2i_dir}/hack/util.sh"

# Exclude these packages from source-to-image explicitely
exclude_pkgs=(
  pkg/cmd
  pkg/config
  pkg/create
  pkg/run
  pkg/test
  pkg/version
)

s2i_dir=$GOPATH/src/github.com/openshift/source-to-image
origin_dir=$GOPATH/src/github.com/openshift/origin
origin_s2i_godep_dir=${origin_dir}/Godeps/_workspace/src/github.com/openshift/source-to-image
s2i_ref=$(cd ${s2i_dir} && git rev-parse --verify HEAD)
s2i_short_ref=$(cd ${s2i_dir} && git rev-parse --short HEAD)
s2i_godeps_ref=$(grep -m1 -A2 'openshift/source-to-image' ${origin_dir}/Godeps/Godeps.json | \
  grep Rev | cut -d ':' -f2 | sed -e 's/"//g' | sed -e 's/^[[:space:]]*//')

pushd "${origin_dir}" >/dev/null
  git checkout -b "s2i-${s2i_short_ref}-bump"
  rm -rf "${origin_s2i_godep_dir}/*"
  cp -R ${s2i_dir}/pkg ${origin_s2i_godep_dir}/.

  # Remove all test files
  find ${origin_s2i_godep_dir} -type f -name '*_test.go' -delete

  # Remove all explicitly excluded packages
  for pkg in "${exclude_pkgs[@]}"; do
    rm -rf "${origin_s2i_godep_dir}/${pkg}"
  done

  # Bump the origin Godeps.json file
  os::util::sed "s/${s2i_godeps_ref}/${s2i_ref}/g" ${origin_dir}/Godeps/Godeps.json

  # Make a commit with proper message
  git add Godeps && git commit -m "bump(github.com/openshift/source-to-image): ${s2i_ref}"
popd

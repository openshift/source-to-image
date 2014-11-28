#!/bin/bash

# This script generates release zips into _output/releases. It requires the openshift/sti-release
# image to be built prior to executing this command.

set -o errexit
set -o nounset
set -o pipefail

STI_ROOT=$(dirname "${BASH_SOURCE}")/..
source "${STI_ROOT}/hack/common.sh"

# Go to the top of the tree.
cd "${STI_ROOT}"

context="${STI_ROOT}/_output/buildenv-context"

# clean existing output
rm -rf "${STI_ROOT}/_output/local/releases"
rm -rf "${STI_ROOT}/_output/local/go/bin"
rm -rf "${context}"
mkdir -p "${context}"
mkdir -p "${STI_ROOT}/_output/local"

# generate version definitions
sti::build::get_version_vars
sti::build::save_version_vars "${context}/sti-version-defs"

# create the input archive
git archive --format=tar -o "${context}/archive.tar" HEAD
tar -rf "${context}/archive.tar" -C "${context}" sti-version-defs
gzip -f "${context}/archive.tar"

# build in the clean environment
cat "${context}/archive.tar.gz" | docker run -i --cidfile="${context}/cid" openshift/sti-release
docker cp $(cat ${context}/cid):/go/src/github.com/openshift/source-to-image/_output/local/releases "${STI_ROOT}/_output/local"

# copy the linux release back to the _output/go/bin dir
releases=$(find _output/local/releases/ -print | grep 'source-to-image-.*-linux-' --color=never)
if [[ $(echo $releases | wc -l) -ne 1 ]]; then
  echo "There should be exactly one Linux release tar in _output/local/releases"
  exit 1
fi
bindir="_output/local/go/bin"
mkdir -p "${bindir}"
tar mxzf "${releases}" -C "${bindir}"

#!/bin/bash

# This script generates release zips into _output/releases

set -o errexit
set -o nounset
set -o pipefail

hackdir=$(CDPATH="" cd $(dirname $0); pwd)

# Set the environment variables required by the build.
. "${hackdir}/config-go.sh"

# Go to the top of the tree.
cd "${STI_REPO_ROOT}"

context="${STI_REPO_ROOT}/_output/buildenv-context"

# clean existing output
rm -rf "${STI_REPO_ROOT}/_output/releases"
rm -rf "${context}"
mkdir -p "${context}"

# generate version definitions
echo "export STI_VERSION=0.1"                            > "${context}/sti-version-defs"
echo "export STI_GITCOMMIT=\"$(sti::build::gitcommit)\"" >> "${context}/sti-version-defs"
echo "export STI_LD_FLAGS=\"$(sti::build::ldflags)\""    >> "${context}/sti-version-defs"

# create the input archive
git archive --format=tar -o "${context}/archive.tar" HEAD
tar -rf "${context}/archive.tar" -C "${context}" sti-version-defs
gzip -f "${context}/archive.tar"

# build in the clean environment
docker build --tag openshift-sti-buildenv "${STI_REPO_ROOT}/hack/buildenv"
cat "${context}/archive.tar.gz" | docker run -i --cidfile="${context}/cid" openshift-sti-buildenv
docker cp $(cat ${context}/cid):/go/src/github.com/openshift/source-to-image/_output/releases "${STI_REPO_ROOT}/_output"

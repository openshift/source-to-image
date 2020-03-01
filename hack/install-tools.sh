#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

echo "Go version: '$(go version)'"

#
# Golint
#

GO111MODULE=off go get golang.org/x/lint/golint

#
# Buildah
#

sudo apt-get update -qq
sudo apt-get install -qq -y software-properties-common
sudo add-apt-repository -y ppa:projectatomic/ppa
sudo apt-get update -qq
sudo apt-get -qq -y install runc buildah

cat <<EOS > /var/tmp/registries.conf
[registries.search]
registries = ['docker.io', 'registry.fedoraproject.org', 'registry.access.redhat.com', 'registry.centos.org', 'quay.io']

[registries.insecure]
registries = []

[registries.block]
registries = []
EOS

sudo mv -v -f /var/tmp/registries.conf /etc/containers/registries.conf

echo "Buldah version: '$(buildah version)'"

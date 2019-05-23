# platform=linux
#
# John Howard Feb 2018. Based on github.com/linuxkit/lcow/pkg/init-lcow/Dockerfile
# This Dockerfile builds initrd.img and rootfs.tar.gz from local opengcs sources.
# It can be used on a Windows machine running in LCOW mode.
#
# Manual steps:
#   git clone https://github.com/Microsoft/opengcs c:\go\src\github.com\Microsoft\opengcs
#   cd c:\go\src\github.com\Microsoft\opengcs
#   docker build --platform=linux -t opengcs .
#   docker run --rm -v c:\target:/build/out opengcs
#   copy c:\target\initrd.img "c:\Program Files\Linux Containers"
#   <TODO: Additional step to generate VHD from rootfs.tar.gz and install>
#   <Restart the docker daemon to pick up the new initrd>

FROM linuxkit/runc:069d5cd3cc4f0aec70e4af53aed5d27a21c79c35 AS runc
FROM busybox AS busybox

FROM golang:1.12.4-alpine3.9
ENV GOPATH=/go PATH=$PATH:/go/bin SRC=/go/src/github.com/Microsoft/opengcs
WORKDIR /build
RUN \
    # Create all the directories
    mkdir -p /target/etc/apk &&  \
    mkdir -p /target/bin && \
    mkdir -p /target/sbin && \
    \
    # Generate base filesystem in /target
    cp -r /etc/apk/* /target/etc/apk/ && \
    apk add --no-cache --initdb -p /target alpine-baselayout busybox e2fsprogs musl && \
    rm -rf /target/etc/apk /target/lib/apk /target/var/cache && \
    \
    # Install the build packages
    apk add --no-cache build-base curl git musl-dev libarchive-tools e2fsprogs file && \
    \
    # Grab udhcpc_config.script
    curl -fSL "https://raw.githubusercontent.com/mirror/busybox/38d966943f5288bb1f2e7219f50a92753c730b14/examples/udhcp/simple.script" -o /target/sbin/udhcpc_config.script && \
    chmod ugo+rx /target/sbin/udhcpc_config.script && \
    \
    # Install gingko for testing
    go get github.com/onsi/ginkgo/ginkgo

COPY --from=runc / /target/
COPY --from=runc /usr/bin/runc /usr/bin/

# Create a test bundle for GCS tests
ENV GCS_TEST_BUNDLE /testbundle
COPY --from=busybox / $GCS_TEST_BUNDLE/rootfs

# Construct a base tar that is added to by the make step below
RUN tar -zcf /build/base.tar.gz -C /target .

# Add the sources for opengcs
COPY . $SRC

# By default, build.
CMD make -f $SRC/Makefile

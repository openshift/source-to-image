#
# This is the release image for building source-to-image.
#
# The standard name for this image is openshift/sti-release
#
FROM registry.redhat.io/ubi9/go-toolset:1.21.13 AS builder

USER root
ENV S2I_VERSION_FILE=/opt/app-root/src/source-to-image/sti-version-defs

RUN mkdir -p /opt/app-root/src/source-to-image

WORKDIR /opt/app-root/src/source-to-image
VOLUME ["/opt/app-root/src/source-to-image"]
# Expect source to be mounted in
CMD ["./hack/build-cross.sh"]

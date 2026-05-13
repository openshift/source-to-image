#
# This is the release image for building source-to-image.
#
# The standard name for this image is openshift/sti-release
#
FROM registry.access.redhat.com/ubi10/go-toolset@sha256:0a1242b10a483946adcf3e35e18f5a19996869468f73deff2c5ce451ce0fa6bc AS builder

USER root
ENV S2I_VERSION_FILE=/opt/app-root/src/source-to-image/sti-version-defs

RUN mkdir -p /opt/app-root/src/source-to-image

WORKDIR /opt/app-root/src/source-to-image
VOLUME ["/opt/app-root/src/source-to-image"]
# Expect source to be mounted in
CMD ["./hack/build-cross.sh"]

#
# This is the release image for building source-to-image.
#
# The standard name for this image is openshift/sti-release
#
FROM registry.access.redhat.com/ubi10/go-toolset@sha256:0c3946b090547d17e44c59f8dab4b8c740889b8b6d4e39e103a7723d73d68a69 AS builder

USER root
ENV S2I_VERSION_FILE=/opt/app-root/src/source-to-image/sti-version-defs

RUN mkdir -p /opt/app-root/src/source-to-image

WORKDIR /opt/app-root/src/source-to-image
VOLUME ["/opt/app-root/src/source-to-image"]
# Expect source to be mounted in
CMD ["./hack/build-cross.sh"]

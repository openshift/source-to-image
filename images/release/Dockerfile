#
# This is the release image for building source-to-image.
#
# The standard name for this image is openshift/sti-release
#
FROM registry.access.redhat.com/ubi10/go-toolset@sha256:557f358c83892a8b94fb72649a99e1c303a625d240547fb97a9cbc1620e808c0 AS builder

USER root
ENV S2I_VERSION_FILE=/opt/app-root/src/source-to-image/sti-version-defs

RUN mkdir -p /opt/app-root/src/source-to-image

WORKDIR /opt/app-root/src/source-to-image
VOLUME ["/opt/app-root/src/source-to-image"]
# Expect source to be mounted in
CMD ["./hack/build-cross.sh"]

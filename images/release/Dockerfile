#
# This is the release image for building source-to-image.
#
# The standard name for this image is openshift/sti-release
#
FROM registry.access.redhat.com/ubi10/go-toolset@sha256:de00e16138966f9fed6bca2d22d28f6cc0d50b26ef6977398e2d8980d80be75f AS builder

USER root
ENV S2I_VERSION_FILE=/opt/app-root/src/source-to-image/sti-version-defs

RUN mkdir -p /opt/app-root/src/source-to-image

WORKDIR /opt/app-root/src/source-to-image
VOLUME ["/opt/app-root/src/source-to-image"]
# Expect source to be mounted in
CMD ["./hack/build-cross.sh"]

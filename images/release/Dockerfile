#
# This is the release image for building source-to-image.
#
# The standard name for this image is openshift/sti-release
#
FROM openshift/origin-release:golang-1.13

ENV S2I_GIT_VERSION="" \
    S2I_GIT_MAJOR="" \
    S2I_GIT_MINOR=""

ENV GOARM=5 \
    GOPATH=/go \
    GOROOT=/usr/local/go \
    S2I_VERSION_FILE=/go/src/github.com/openshift/source-to-image/sti-version-defs
ENV PATH=$PATH:$GOROOT/bin:$GOPATH/bin

WORKDIR /go/src/github.com/openshift/source-to-image
COPY . .

ENV GOARCH="amd64"

USER root

RUN make

FROM registry.access.redhat.com/ubi8/ubi

ENV GOARCH="amd64" \
    S2I_WORKING_DIR="/var/lib/source-to-image" \
    S2I_VERSION_FILE=/go/src/github.com/openshift/source-to-image/sti-version-defs

COPY --from=0 /go/src/github.com/openshift/source-to-image/_output/local/bin/linux/${GOARCH}/s2i /usr/local/bin/s2i

RUN mkdir /var/lib/source-to-image \
    && chmod 0666 /var/lib/source-to-image

# Inform dependency in the PR.
RUN yum install -y git && \
    yum clean all \
    ; \
    rm -rfv /var/cache /var/log/dnf* /var/log/yum.*

WORKDIR /var/lib/source-to-image

USER 1001

ENTRYPOINT [ "/usr/local/bin/s2i" ]

VOLUME /var/run/docker.sock

VOLUME /var/lib/source-to-image
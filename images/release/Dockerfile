#
# This is the release image for building source-to-image.
#
# The standard name for this image is openshift/sti-release
#
FROM registry.redhat.io/ubi8/ubi

ENV VERSION=1.12.5 \
    GOARM=5 \
    GOPATH=/go \
    GOROOT=/usr/local/go \
    S2I_VERSION_FILE=/go/src/github.com/openshift/source-to-image/sti-version-defs
ENV PATH=$PATH:$GOROOT/bin:$GOPATH/bin

RUN yum install -y gcc zip && \
    yum clean all && \
    curl https://storage.googleapis.com/golang/go$VERSION.linux-amd64.tar.gz | tar -C /usr/local -xzf - && \
    touch /sti-build-image && \
    mkdir -p /go/src/github.com/openshift/source-to-image

WORKDIR /go/src/github.com/openshift/source-to-image
VOLUME ["/go/src/github.com/openshift/source-to-image"]
# Expect source to be mounted in
CMD ["./hack/build-cross.sh"]

FROM openshift/origin-release:golang-1.13 AS builder

ENV S2I_GIT_VERSION="" \
    S2I_GIT_MAJOR="" \
    S2I_GIT_MINOR=""

ENV GOARCH="amd64"

COPY . $GOPATH/src/github.com/openshift/source-to-image

RUN cd $GOPATH/src/github.com/openshift/source-to-image && \
    make && \
    install _output/local/bin/linux/${GOARCH}/s2i /usr/local/bin

#
# Runner Image
#

FROM registry.redhat.io/ubi8/ubi

COPY --from=builder /usr/local/bin/s2i /usr/local/bin

ENTRYPOINT [ "/usr/local/bin/s2i" ]
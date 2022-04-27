FROM openshift/origin-release:golang-1.17 AS builder

ENV S2I_GIT_VERSION="" \
    S2I_GIT_MAJOR="" \
    S2I_GIT_MINOR=""


WORKDIR /tmp/source-to-image
COPY . .

USER root

RUN make && cp _output/local/bin/linux/$(go env GOARCH)/s2i _output/local/go/bin/s2i

#
# Runner Image
#

FROM registry.redhat.io/ubi8/ubi

COPY --from=builder /tmp/source-to-image/_output/local/go/bin/s2i  /usr/local/bin/s2i

USER 1001

ENTRYPOINT [ "/usr/local/bin/s2i" ]

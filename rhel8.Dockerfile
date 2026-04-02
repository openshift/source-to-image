FROM registry.redhat.io/ubi8/go-toolset@sha256:b83390c62618db65aea35507285d7f1c79e609952cded9eef9f7f01b0bc1a45e AS builder

ENV S2I_GIT_VERSION="1.6.0" \
    S2I_GIT_MAJOR="1" \
    S2I_GIT_MINOR="6"

ENV GOEXPERIMENT=strictfipsruntime

COPY . .

RUN CGO_ENABLED=1 GO111MODULE=on go build -a -mod=vendor -ldflags="-s -w" -tags="strictfipsruntime exclude_graphdriver_btrfs" -o /tmp/s2i ./cmd/s2i


FROM registry.redhat.io/ubi8-minimal@sha256:815d4b55a154d9863c84f37298f4ebd2ae131bb244824996824138fc6c4b4b7d

COPY --from=builder /tmp/s2i /usr/local/bin/s2i

USER 1001

ENTRYPOINT [ "/usr/local/bin/s2i" ]

LABEL \
    name="source-to-image/source-to-image-rhel8" \
    description="Source-to-Image is a builder image" \
    summary="Source-to-Image is a builder image" \
    version="v1.6.0" \
    vendor="Red Hat, Inc." \
    com.redhat.component="source-to-image-container" \
    cpe="cpe:/a:redhat:source_to_image:1.6::el8" \
    maintainer="openshift-builds@redhat.com" \
    distribution-scope="public" \
    release="v1.6.0" \
    url="https://github.com/openshift/source-to-image" \
    io.k8s.description="Source-to-Image is a builder image" \
    io.k8s.display-name="Source-to-Image" \
    io.openshift.tags="source-to-image,s2i" \
    io.openshift.maintainer.product="OpenShift Container Platform" \
    io.openshift.maintainer.component="Source-to-Image"

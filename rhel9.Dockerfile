FROM registry.access.redhat.com/ubi8/go-toolset:1.22 AS builder
ENV S2I_GIT_VERSION="1.5.0" \
    S2I_GIT_MAJOR="1" \
    S2I_GIT_MINOR="5"

ENV GOEXPERIMENT=strictfipsruntime

COPY . .

RUN CGO_ENABLED=1 GO111MODULE=on go build -a -mod=vendor -ldflags="-s -w" -tags="strictfipsruntime exclude_graphdriver_btrfs" -o /tmp/s2i ./cmd/s2i


FROM registry.redhat.io/ubi9/ubi-minimal:9.6

COPY --from=builder /tmp/s2i /usr/local/bin/s2i

USER 1001

ENTRYPOINT [ "/usr/local/bin/s2i" ]

LABEL \
    name="source-to-image/source-to-image-rhel9" \
    description="Source-to-Image is a builder image" \
    summary="Source-to-Image is a builder image" \
    version="1.6.0" \
    vendor="Red Hat, Inc." \
    com.redhat.component="source-to-image-container" \
    maintainer="openshift-builds@redhat.com" \
    io.k8s.description="Source-to-Image is a builder image" \
    io.k8s.display-name="Source-to-Image" \
    io.openshift.tags="source-to-image,s2i" \
    io.openshift.maintainer.product="OpenShift Container Platform" \
    io.openshift.maintainer.component="Source-to-Image"

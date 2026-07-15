FROM registry.redhat.io/ubi9/go-toolset@sha256:a2ba4645e7c424b08aa83ed7792e279683b0d33acbc5131b18183fd21e336c55 AS builder
ENV S2I_GIT_VERSION="1.6.2" \
    S2I_GIT_MAJOR="1" \
    S2I_GIT_MINOR="6"

ENV GOEXPERIMENT=strictfipsruntime

COPY . .

RUN CGO_ENABLED=1 GO111MODULE=on go build -a -mod=vendor -ldflags="-s -w" -tags="strictfipsruntime exclude_graphdriver_btrfs" -o /tmp/s2i ./cmd/s2i


FROM registry.redhat.io/ubi9-minimal@sha256:062c52ff973065752b0965787649db2bcf551a6c727a00e95a3eb42cebadbdab

COPY --from=builder /tmp/s2i /usr/local/bin/s2i

USER 1001

ENTRYPOINT [ "/usr/local/bin/s2i" ]

LABEL \
    name="source-to-image/source-to-image-rhel9" \
    description="Source-to-Image is a builder image" \
    summary="Source-to-Image is a builder image" \
    version="v1.6.2" \
    vendor="Red Hat, Inc." \
    com.redhat.component="source-to-image-container" \
    cpe="cpe:/a:redhat:source_to_image:1.6::el8" \
    maintainer="openshift-builds@redhat.com" \
    distribution-scope="public" \
    release="v1.6.2" \
    url="https://github.com/openshift/source-to-image" \
    io.k8s.description="Source-to-Image is a builder image" \
    io.k8s.display-name="Source-to-Image" \
    io.openshift.tags="source-to-image,s2i" \
    io.openshift.maintainer.product="OpenShift Container Platform" \
    io.openshift.maintainer.component="Source-to-Image"

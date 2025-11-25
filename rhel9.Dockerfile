FROM registry.redhat.io/ubi10/go-toolset:1.25.3 AS builder
ENV S2I_GIT_VERSION="1.5.2" \
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
    version="v1.5.2" \
    vendor="Red Hat, Inc." \
    com.redhat.component="source-to-image-container" \
    cpe="cpe:/a:redhat:source_to_image:1.5::el8" \
    maintainer="openshift-builds@redhat.com" \
    distribution-scope="public" \
    release="v1.5.2" \
    url="https://catalog.redhat.com/en/software/container-stacks/detail/5ec54a2e110f56bd24f2ddc7" \
    io.k8s.description="Source-to-Image is a builder image" \
    io.k8s.display-name="Source-to-Image" \
    io.openshift.tags="source-to-image,s2i" \
    io.openshift.maintainer.product="OpenShift Container Platform" \
    io.openshift.maintainer.component="Source-to-Image"

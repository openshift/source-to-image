FROM registry.redhat.io/ubi9/go-toolset@sha256:15c3098dc4639e8a0e6a1bc77b50964513a1d76dccae4b07b9808101c5addd04 AS builder

ENV S2I_GIT_VERSION="1.6.2" \
    S2I_GIT_MAJOR="1" \
    S2I_GIT_MINOR="6"

ENV GOEXPERIMENT=strictfipsruntime

COPY . .

RUN CGO_ENABLED=1 GO111MODULE=on go build -a -mod=vendor -ldflags="-s -w" -tags="strictfipsruntime exclude_graphdriver_btrfs" -o /tmp/s2i ./cmd/s2i


FROM registry.redhat.io/ubi8-minimal@sha256:128021168edb5b3013258601a2dffe93fddfed91cf996c08c76a24dfdcd6de13

COPY --from=builder /tmp/s2i /usr/local/bin/s2i

USER 1001

ENTRYPOINT [ "/usr/local/bin/s2i" ]

LABEL \
    name="source-to-image/source-to-image-rhel8" \
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

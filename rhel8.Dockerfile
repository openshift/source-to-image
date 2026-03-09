FROM registry.redhat.io/ubi8/go-toolset@sha256:10bd7b95a28b8f19c396dbd357c537e62ed9c97145e769b63c93cc3ea7fd033e AS builder

ENV S2I_GIT_VERSION="1.6.0" \
    S2I_GIT_MAJOR="1" \
    S2I_GIT_MINOR="6"

ENV GOEXPERIMENT=strictfipsruntime

COPY . .

RUN CGO_ENABLED=1 GO111MODULE=on go build -a -mod=vendor -ldflags="-s -w" -tags="strictfipsruntime exclude_graphdriver_btrfs" -o /tmp/s2i ./cmd/s2i


FROM registry.redhat.io/ubi8-minimal@sha256:b880e16b888f47bc3fae64e67cd9776b24372f2e7ec2051f5a9386de6f5a75ac

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

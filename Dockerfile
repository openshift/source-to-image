FROM registry.redhat.io/ubi8/go-toolset:1.21 AS builder

ENV S2I_GIT_VERSION="1.4.0" \
    S2I_GIT_MAJOR="1" \
    S2I_GIT_MINOR="4"

COPY . .

RUN CGO_ENABLED=0 go build -a -ldflags="-s -w" -o /tmp/s2i ./cmd/s2i

#
# Runner Image
#

FROM registry.redhat.io/ubi8/ubi-minimal:8.10

COPY --from=builder /tmp/s2i /usr/local/bin/s2i

USER 1001

ENTRYPOINT [ "/usr/local/bin/s2i" ]

LABEL \
    name="source-to-image/source-to-image" \
    description="Source-to-Image is a builder image" \
    summary="Source-to-Image is a builder image" \
    version="1.4.0" \
    architecture="${GOARCH}" \
    vendor="Red Hat, Inc." \
    com.redhat.component="source-to-image-container" \
    maintainer="openshift-builds@redhat.com" \
    io.k8s.description="Source-to-Image is a builder image" \
    io.k8s.display-name="Source-to-Image" \
    io.openshift.tags="source-to-image,s2i" \
    io.openshift.maintainer.product="OpenShift Container Platform" \
    io.openshift.maintainer.component="Source-to-Image" \

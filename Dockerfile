FROM registry.access.redhat.com/ubi8/go-toolset@sha256:be796155c0908cd48375bf1f7150036bcd3ad415dfb6cae135f1cf184d61964c AS builder

ENV S2I_GIT_VERSION="1.5.0" \
    S2I_GIT_MAJOR="1" \
    S2I_GIT_MINOR="5"

ENV GOEXPERIMENT=strictfipsruntime

COPY . .

RUN CGO_ENABLED=1 GO111MODULE=on go build -a -mod=vendor -ldflags="-s -w" -tags="strictfipsruntime exclude_graphdriver_btrfs" -o /tmp/s2i ./cmd/s2i


FROM registry.access.redhat.com/ubi8@sha256:881aaf5fa0d1f85925a1b9668a1fc7f850a11ca30fd3e37ea194db4edff892a5

COPY --from=builder /tmp/s2i /usr/local/bin/s2i

USER 1001

ENTRYPOINT [ "/usr/local/bin/s2i" ]

LABEL \
    name="source-to-image/source-to-image" \
    description="Source-to-Image is a builder image" \
    summary="Source-to-Image is a builder image" \
    version="1.5.0" \
    vendor="Red Hat, Inc." \
    com.redhat.component="source-to-image-container" \
    maintainer="openshift-builds@redhat.com" \
    io.k8s.description="Source-to-Image is a builder image" \
    io.k8s.display-name="Source-to-Image" \
    io.openshift.tags="source-to-image,s2i" \
    io.openshift.maintainer.product="OpenShift Container Platform" \
    io.openshift.maintainer.component="Source-to-Image"
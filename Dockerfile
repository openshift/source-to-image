FROM registry.redhat.io/ubi8/go-toolset:1.21 AS builder

ENV S2I_GIT_VERSION="" \
    S2I_GIT_MAJOR="" \
    S2I_GIT_MINOR=""

COPY . .

RUN CGO_ENABLED=0 go build -a -ldflags="-s -w" -o /tmp/s2i ./cmd/s2i

#
# Runner Image
#

FROM registry.redhat.io/ubi8/ubi-minimal:8.10

COPY --from=builder /tmp/s2i /usr/local/bin/s2i

USER 1001

ENTRYPOINT [ "/usr/local/bin/s2i" ]

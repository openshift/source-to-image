# This file creates a standard build environment for building STI
FROM  fedora:21

RUN yum install -y git hg golang golang-pkg-darwin golang-pkg-windows && yum clean all

ENV GOPATH /go
# Get the code coverage tool and godep
RUN go get code.google.com/p/go.tools/cmd/cover github.com/tools/godep

# Mark this as a sti-build container
RUN touch /sti-build-image

WORKDIR /go/src/github.com/openshift/source-to-image

ENV OS_CROSSPLATFORMS \
  darwin/amd64 \
  windows/amd64
# (set an explicit GOARM of 5 for maximum compatibility)
ENV GOARM 5
ENV GOOS    linux
ENV GOARCH  amd64
ENV PATH $PATH:$GOROOT/bin:$GOPATH/bin

# Expect a tar with the source of STI (and /os-version-defs in the root)
CMD tar -xzf - && ./hack/buildenv/build.sh

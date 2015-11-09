#
# This is the release image for building source-to-image.
#
# The standard name for this image is openshift/sti-release
#
FROM openshift/origin-base

RUN yum install -y zip hg golang golang-pkg-darwin-amd64 golang-pkg-windows-amd64 golang-pkg-linux-386 && yum clean all

ENV GOPATH /go

# Get the code coverage tool and godep
RUN go get golang.org/x/tools/cmd/cover github.com/tools/godep

# Mark this as a sti-build container
RUN touch /sti-build-image

WORKDIR /go/src/github.com/openshift/source-to-image

# (set an explicit GOARM of 5 for maximum compatibility)
ENV GOARM 5
ENV PATH $PATH:$GOROOT/bin:$GOPATH/bin

ENV STI_VERSION_FILE /go/src/github.com/openshift/source-to-image/sti-version-defs

# Expect a tar with the source of STI (and /sti-version-defs in the root)
CMD tar mxzf - && hack/build-cross.sh

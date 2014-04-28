FROM fedora
MAINTAINER Paul Morie <pmorie@redhat.com>

ENV GOPATH /go
RUN yum install -y golang git hg gcc libselinux-devel && yum clean all
RUN mkdir -p $GOPATH && echo $GOPATH >> ~/.bash_profile

ADD     . /go/src/github.com/pmorie/go-sti
WORKDIR   /go/src/github.com/pmorie/go-sti
RUN \
   go get ./... && \
   go install github.com/pmorie/go-sti/sti && \
   /bin/cp /go/bin/sti /bin/sti && \
   rm -rf $GOPATH

CMD ["/bin/sti"]

FROM openshift/origin-release:golang-1.13 AS builder

ENV S2I_GIT_VERSION="" \
    S2I_GIT_MAJOR="" \
    S2I_GIT_MINOR=""

WORKDIR /tmp/source-to-image
COPY . .

RUN make

#
# Runner Image
#

FROM registry.access.redhat.com/ubi8/ubi

ENV GOARCH="amd64" \
    _BUILDAH_STARTED_IN_USERNS="" \
    BUILDAH_ISOLATION="chroot" \
    S2I_HOME="/s2i" \
    S2I_CONTAINER_MANAGER="buildah"

RUN useradd --uid 1000 --groups root --home-dir $S2I_HOME s2i && \
    mkdir /src && \
    chown s2i:s2i /src

RUN echo -en "\
[AppStream]\n\
name=CentOS AppStream\n\
mirrorlist=http://mirrorlist.centos.org/?release=8&arch=x86_64&repo=AppStream&infra=\n\
# baseurl=http://mirror.centos.org/$contentdir/$releasever/AppStream/$basearch/os/\n\
gpgcheck=1\n\
enabled=1\n\
gpgkey=https://www.centos.org/keys/RPM-GPG-KEY-CentOS-Official\n\
" > /etc/yum.repos.d/appstream.repo && \
    cat /etc/yum.repos.d/appstream.repo && \
    yum -y reinstall shadow-utils && \
    yum -y install buildah fuse-overlayfs fuse3 git && \
    rm -rf /var/cache /var/log/dnf* /var/log/yum.*

RUN sed -i \
        -e 's|^#mount_program|mount_program|g' \
        -e '/additionalimage.*/a "/var/lib/shared",' \
        -e 's|^mountopt[[:space:]]*=.*$|mountopt = "nodev,fsync=0"|g' \
        /etc/containers/storage.conf && \
    mkdir -p \
        $S2I_HOME/.config/containers \
        $S2I_HOME/.local/share/containers/storage \
        /var/run/containers \
        /var/lib/containers/storage \
        /var/lib/shared/overlay-{images,layers} && \
    touch /var/lib/shared/overlay-images/images.lock /var/lib/shared/overlay-layers/layers.lock && \
    cp -v /etc/containers/storage.conf $S2I_HOME/.config/containers/storage.conf && \
    cat $S2I_HOME/.config/containers/storage.conf |grep -v '^#' |grep -v '^$' && \
    chown -R s2i:s2i \
        $S2I_HOME/.config \
        $S2I_HOME/.local \
        /var/run/containers \
        /var/lib/{containers,shared}

COPY --from=builder /tmp/source-to-image/_output/local/bin/linux/${GOARCH}/s2i  /usr/local/bin/s2i

# container image storage location
VOLUME /var/lib/containers/storage
# additional images storage location
VOLUME /var/lib/shared
# default location for mounting the project repository
VOLUME /src

WORKDIR /src

USER s2i

ENTRYPOINT [ "/usr/local/bin/s2i" ]

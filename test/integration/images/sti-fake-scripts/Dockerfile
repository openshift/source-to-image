#
# This is fake image used for testing STI. It tests scripts baked inside image.
#
FROM busybox

RUN mkdir -p /sti-fake/src && \
    mkdir -p /tmp/scripts && \
# FIXME: The 'default' user has conflicting UID (1000). This should be fixed in
#        how source-to-image handles UID's during isolation.
#
    deluser default

WORKDIR /

ADD scripts/.sti/bin/ /tmp/scripts/

# Scripts are already in the image and this is their location
ENV STI_SCRIPTS_URL image:///tmp/scripts/

#
# This is basic fake image used for testing STI.
#
FROM busybox

RUN mkdir -p /sti-fake/src && \
    rm /bin/tar && \
# FIXME: The 'default' user has conflicting UID (1000). This should be fixed in
#        how source-to-image handles UID's during isolation.
#
    deluser default

WORKDIR /

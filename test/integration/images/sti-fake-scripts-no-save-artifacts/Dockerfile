#
# This is fake image used for testing STI. It tests scripts baked inside image.
#
FROM busybox

RUN mkdir -p /sti-fake/src && \
    mkdir -p /tmp/scripts

WORKDIR /

ADD scripts/.sti/bin/run /tmp/scripts/
ADD scripts/.sti/bin/assemble /tmp/scripts/

# Scripts are already in the image and this is their location
LABEL io.openshift.sti.scripts-url=image:///tmp/scripts/

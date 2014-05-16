FROM sti_test/sti-fake

RUN mkdir -p /sti-fake && \
    groupadd -r fakeuser -f -g 433 && \
    useradd -u 431 -r -g fakeuser -d /sti-fake -s /sbin/nologin -c "Fake User" fakeuser && \
    chown -R fakeuser:fakeuser /sti-fake

USER fakeuser
ENV STI_SCRIPTS_URL https://raw.githubusercontent.com/openshift/docker-source-to-images/master/test_images/sti-fake/.sti/bin
                
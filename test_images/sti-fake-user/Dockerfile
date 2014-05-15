FROM sti_test/sti-fake

RUN mkdir -p /sti-fake && \
    groupadd -r fakeuser -f -g 433 && \
    useradd -u 431 -r -g fakeuser -d /sti-fake -s /sbin/nologin -c "Fake User" fakeuser && \
    chown -R fakeuser:fakeuser /sti-fake

USER fakeuser
ENV STI_SCRIPTS_URL https://raw.githubusercontent.com/bparees/docker-source-to-images/remote_scripts/test_images/sti-fake/sti_scripts
                
FROM sti_test/sti-fake

RUN mkdir -p /sti-fake && \
    adduser -u 431 -h /sti-fake -s /sbin/nologin -D fakeuser && \
    chown -R fakeuser /sti-fake

USER fakeuser

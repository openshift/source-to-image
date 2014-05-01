FROM pmorie/sti-fake

RUN mkdir -p /sti-fake && \
    groupadd -r fakeuser -f -g 433 && \
    useradd -u 431 -r -g fakeuser -d /sti-fake -s /sbin/nologin -c "Fake User" fakeuser && \
    chown -R fakeuser:fakeuser /sti-fake

USER fakeuser

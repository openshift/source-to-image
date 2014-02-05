FROM fedora:20
RUN mkdir /sti-fake-builder
ADD ./prepare /usr/bin/
ADD ./run /usr/bin/
ADD ./save-artifacts /usr/bin/
RUN chmod +x /usr/bin/prepare /usr/bin/run /usr/bin/save-artifacts
WORKDIR /

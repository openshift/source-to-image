FROM mattdm/fedora:f20
RUN mkdir /wharfie-fake
ADD ./prepare /usr/bin/
ADD ./run /usr/bin/
RUN chmod +x /usr/bin/prepare /usr/bin/run
WORKDIR /

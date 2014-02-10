FROM fedora:20
RUN yum -y install git python-pip
RUN pip install git+https://github.com/openshift/docker-source-to-images
VOLUME /var/run/
VOLUME /usr/src
CMD sti

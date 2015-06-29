package templates

const Dockerfile = `
# {{.ImageName}}
FROM openshift/base-centos7

# TODO: Put the maintainer name in the image metadata
# MAINTAINER Your Name <your@email.com>

# TODO: Rename the builder environment variable to inform users about application you provide them
# ENV BUILDER_VERSION 1.0

# TODO: Set labels used in OpenShift to describe the builder image
#LABEL io.k8s.description="Platform for building xyz" \
#      io.k8s.display-name="builder x.y.z" \
#      io.openshift.expose-services="8080:http" \
#      io.openshift.tags="builder,x.y.z,etc."

# TODO: Install required packages here:
# RUN yum install -y ... && yum clean all -y

# This default user is created in the openshift/base-centos7 image
USER 1001

# TODO (optional): Copy the builder files into /opt/openshift
# COPY ./<builder_folder>/ /opt/openshift/

# TODO: Copy the S2I scripts to /usr/local/sti, since openshift/base-centos7 image sets io.s2i.scripts-url label that way, or update that label
# COPY ./.sti/bin/ /usr/local/sti

# TODO: Drop the root user and make the content of /opt/openshift owned by user 1001
# RUN chown -R 1001:1001 /opt/openshift

# TODO: Set the default port for applications built using this image
# EXPOSE 8080

# TODO: Set the default CMD for the image
# CMD ["usage"]
`

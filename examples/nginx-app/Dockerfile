# nginx-centos7
# Here you can use whatever image base is relevant for your application.
FROM openshift/base-centos7

# Here you can specify the maintainer for the image that you're building
MAINTAINER Victor Palade <ipalade@redhat.com>

# Export the environment variable that provides information about the application.
# Replace this with the according version for your application.
ENV NGINX_VERSION=1.10.1

# Set the labels that are used for OpenShift to describe the builder image.
LABEL io.k8s.description="Nginx Webserver" \
    io.k8s.display-name="Nginx 1.10.1" \
    io.openshift.expose-services="8080:http" \
    io.openshift.tags="builder,webserver,html,nginx" \
    authors="Victor Palade <ipalade@redhat.com>,Luis Fernando Gommes <dev@luiscoms.com.br>"

# Install our Nginx package and clean the yum cache so that we don't have any
# cached files waste space.
RUN yum install -y epel-release  && \
    yum install -y --setopt=tsflags=nodocs nginx && \
    yum clean all

# Create dir to mantain pidfile
RUN mkdir -p /opt/app-root/run/

# We will change the default port for nginx (It's required if you plan on
# running images as non-root user).
RUN sed -i 's|pid /run/nginx.pid;|pid /opt/app-root/run/nginx.pid;|; \
         s|user nginx;||; \
         s|error_log.*|error_log /dev/stderr;|; \
         s|access_log.*|access_log /dev/stdout main;|; \
         s|/usr/share/nginx/html|/opt/app-root/src|; \
         s|80|8080|;' /etc/nginx/nginx.conf

# Copy the S2I scripts to /usr/libexec/s2i, since openshift/base-centos7 image sets io.openshift.s2i.scripts-url label that way, or update that label
COPY ./.s2i/bin/ $STI_SCRIPTS_PATH

# Drop the root user and make the content of /opt/app-root owned by user nginx
RUN chown -R nginx:nginx /opt/app-root

# Run container as nginx user
USER nginx

# Set the default port for applications built using this image
EXPOSE 8080

# Modify the usage script in your application dir to inform the user how to run
# this image.
CMD ["/usr/libexec/s2i/usage"]

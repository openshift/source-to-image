FROM mattdm/fedora:latest
RUN mkdir /usr/mock
RUN yum install -y ruby-devel rubygems gcc openssl-devel && yum clean all
RUN gem install webrick
ADD ./mock_server.rb /usr/mock/
ADD ./prepare /usr/bin/
ADD ./run /usr/bin/
EXPOSE 8080
RUN chmod +x /usr/bin/prepare /usr/bin/run /usr/mock/mock_server.rb

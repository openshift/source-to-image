#
# This is basic fake image used for testing STI.
#
FROM busybox

RUN mkdir -p /sti-fake/src

WORKDIR /

# Need to serve the scripts from localhost so any potential changes to the
# scripts are made available for integration testing.
#
# Port 23456 must match the port used in the http server in STI's
# integration_test.go
ENV STI_SCRIPTS_URL http://127.0.0.1:23456/.sti/bin

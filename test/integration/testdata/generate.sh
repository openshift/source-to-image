#!/bin/sh
# Generate new 127.0.0.1.crt, 127.0.0.1.key, client.crt, client.key, ca.crt,
# and ca.key.  The ca.key isn't kept in source control because we can just
# make a new one and make a new version of everything that it signed.
config=`mktemp`
trap 'rm -f $config' EXIT
openssl req -config $config -new -nodes -newkey rsa:2048 -keyout ca.key -x509 -out ca.crt -subj "/CN=Test CA" -days 730 -addext basicConstraints=CA:TRUE -addext subjectKeyIdentifier=hash
openssl req -config $config -new -nodes -newkey rsa:2048 -keyout 127.0.0.1.key -x509 -out 127.0.0.1.crt -subj "/CN=127.0.0.1" -CA ca.crt -CAkey ca.key -days 730 -addext basicConstraints=CA:FALSE -addext subjectAltName=IP:127.0.0.1,IP:::1 -addext subjectKeyIdentifier=hash -addext authorityKeyIdentifier=keyid -addext extendedKeyUsage=serverAuth
openssl req -config $config -new -nodes -newkey rsa:2048 -keyout client.key -x509 -out client.crt -subj "/CN=client" -CA ca.crt -CAkey ca.key -days 730 -addext basicConstraints=CA:FALSE -addext subjectKeyIdentifier=hash -addext authorityKeyIdentifier=keyid -addext extendedKeyUsage=clientAuth

#!/bin/sh
pushd sti-fake
docker build -t sti_test/sti-fake .
cd ../sti-fake-user
docker build -t sti_test/sti-fake-user .
cd ../sti-fake-broken
docker build -t sti_test/sti-fake-broken .
popd

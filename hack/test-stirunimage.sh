#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

STI_ROOT=$(dirname "${BASH_SOURCE}")/..

function time_now()
{
  echo $(date +%s000)
}

function cleanup()
{
    set +e
    rm -f  "${STI_ROOT}/hack/sti-run.log"
    # use sigint so that sti post processing will remove docker container
    if [ -z "$KILLDONE" ]; then
	kill -2 "${STI_PID}"
    fi
    echo
    echo "Complete"
}

set +e
img_count=$(docker images | grep -c sti_test/sti-fake)
set -e

if [ "${img_count}" != "10" ]; then
    echo "You do not have necessary test images, be sure to run 'hack/build-test-images.sh' beforehand."
    exit 1
fi

trap cleanup EXIT SIGINT

echo
echo 
echo

sti build git://github.com/bparees/openshift-jee-sample openshift/wildfly-8-centos test-jee-app --run=true &> "${STI_ROOT}/hack/sti-run.log" &
export STI_PID=$!
TIME_SEC=1000
TIME_MIN=$((60 * $TIME_SEC))
max_wait=10*TIME_MIN
echo "waiting up to ${max_wait}"
expire=$(($(time_now) + $max_wait))

set +e
while [[ $(time_now) -lt $expire ]]; do
    grep  "as a result of the --run=true option" "${STI_ROOT}/hack/sti-run.log"
    if [ $? -eq 0 ]; then
      echo "[INFO] Success running command sti --run=true"
      
      # use sigint so that sti post processing will remove docker container
      kill -2 "${STI_PID}"
      export KILLDONE="killed"
      sleep 30
      docker ps -a | grep test-jee-app

      if [ $? -eq 1 ]; then
	  echo "[INFO] Success terminating associated docker container"
	  exit 0
      else
	  echo "[INFO] Associated docker container still found, review docker ps -a output above, and here is the dump of ${STI_ROOT}/hack/sti-run.log"
	  cat "${STI_ROOT}/hack/sti-run.log"
	  exit 1
      fi
    fi
    sleep 1
done

echo "[INFO] Problem with sti --run=true, dumping ${STI_ROOT}/hack/sti-run.log"
cat "${STI_ROOT}/hack/sti-run.log"
set -e
exit 1

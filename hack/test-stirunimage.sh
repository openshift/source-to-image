#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

S2I_ROOT=$(dirname "${BASH_SOURCE}")/..
export KILLDONE=""

function time_now()
{
  echo $(date +%s000)
}

mkdir -p /tmp/sti
WORK_DIR=$(mktemp -d /tmp/sti/test-work.XXXX)
NEEDKILL="yes"
S2I_PID=""
function cleanup()
{
    set +e
    #some failures will exit the shell script before check_result() can dump the logs (ssh seems to be such a case)
    if [ -a "${WORK_DIR}/ran-clean" ]; then
	echo "Cleaning up working dir ${WORK_DIR}"
    else
	echo "Dumping logs since did not run successfully before cleanup of ${WORK_DIR} ..."
	cat /tmp/test-work*/*
    fi
    rm -rf ${WORK_DIR}
    # use sigint so that s2i post processing will remove docker container
    if [ -n "${NEEDKILL}" ]; then
	if [ -n "${S2I_PID}" ]; then
	    kill -2 "${S2I_PID}"
	fi
    fi
    echo
    echo "Complete"
}

function check_result() {
  local result=$1
  if [ $result -eq 0 ]; then
      echo
      echo "TEST PASSED"
      echo
      if [ -n "${2}" ]; then
	  rm $2
      fi
  else
      echo
      echo "TEST FAILED ${result}"
      echo
      cat $2
      cleanup
      exit $result
  fi
}

function test_debug() {
    echo
    echo $1
    echo
}

set +e
img_count=$(docker images | grep -c sti_test/sti-fake)
set -e

if [ "${img_count}" != "10" ]; then
    echo "You do not have necessary test images, be sure to run 'hack/build-test-images.sh' beforehand."
    exit 1
fi

trap cleanup EXIT SIGINT

echo "working dir:  ${WORK_DIR}"
pushd ${WORK_DIR}

test_debug "cloning source into working dir"

git clone git://github.com/openshift/cakephp-ex &> "${WORK_DIR}/s2i-git-clone.log"
check_result $? "${WORK_DIR}/s2i-git-clone.log"

test_debug "s2i build with relative path without file://"

s2i build cakephp-ex openshift/php-55-centos7 test &> "${WORK_DIR}/s2i-rel-noproto.log"
check_result $? "${WORK_DIR}/s2i-rel-noproto.log"

test_debug "s2i build with relative path with file://"

s2i build file://./cakephp-ex openshift/php-55-centos7 test &> "${WORK_DIR}/s2i-rel-proto.log"
check_result $? "${WORK_DIR}/s2i-rel-proto.log"

popd

test_debug "s2i build with absolute path with file://"

s2i build "file://${WORK_DIR}/cakephp-ex" openshift/php-55-centos7 test &> "${WORK_DIR}/s2i-abs-proto.log"
check_result $? "${WORK_DIR}/s2i-abs-proto.log"

test_debug "s2i build with absolute path without file://"

s2i build "${WORK_DIR}/cakephp-ex" openshift/php-55-centos7 test &> "${WORK_DIR}/s2i-abs-noproto.log"
check_result $? "${WORK_DIR}/s2i-abs-noproto.log"

## don't do ssh tests here because credentials are needed (even for the git user), which
## don't exist in the vagrant/jenkins setup

test_debug "s2i build with non-git repo file location"

rm -rf "${WORK_DIR}/cakephp-ex/.git"
s2i build "${WORK_DIR}/cakephp-ex" openshift/php-55-centos7 test --loglevel=5 &> "${WORK_DIR}/s2i-non-repo.log"
check_result $? ""
grep "Copying sources" "${WORK_DIR}/s2i-non-repo.log"
check_result $? "${WORK_DIR}/s2i-non-repo.log"

test_debug "s2i usage"

s2i usage openshift/ruby-20-centos7 &> "${WORK_DIR}/s2i-usage.log"
check_result $? ""
grep "Sample invocation" "${WORK_DIR}/s2i-usage.log"
check_result $? "${WORK_DIR}/s2i-usage.log"

test_debug "s2i build with git proto"

s2i build git://github.com/openshift/cakephp-ex openshift/php-55-centos7 test --run=true &> "${WORK_DIR}/s2i-git-proto.log" &
check_result $? "${WORK_DIR}/s2i-git-proto.log"

test_debug "s2i build with --run==true option"
s2i build git://github.com/bparees/openshift-jee-sample openshift/wildfly-90-centos7 test-jee-app --run=true &> "${WORK_DIR}/s2i-run.log" &
S2I_PID=$!
TIME_SEC=1000
TIME_MIN=$((60 * $TIME_SEC))
max_wait=15*TIME_MIN
echo "Waiting up to ${max_wait} for the build to finish ..."
expire=$(($(time_now) + $max_wait))

set +e
while [[ $(time_now) -lt $expire ]]; do
    grep  "as a result of the --run=true option" "${WORK_DIR}/s2i-run.log"
    if [ $? -eq 0 ]; then
      echo "[INFO] Success running command s2i --run=true"

      # use sigint so that s2i post processing will remove docker container
      kill -2 "${S2I_PID}"
      NEEDKILL=""
      sleep 30
      docker ps -a | grep test-jee-app

      if [ $? -eq 1 ]; then
	     echo "[INFO] Success terminating associated docker container"
	     touch "${WORK_DIR}/ran-clean"
	     exit 0
      else
	     echo "[INFO] Associated docker container still found, review docker ps -a output above, and here is the dump of ${WORK_DIR}/s2i-run.log"
	     cat "${WORK_DIR}/s2i-run.log"
	     exit 1
      fi
    fi
    sleep 1
done

echo "[INFO] Problem with s2i --run=true, dumping ${WORK_DIR}/s2i-run.log"
cat "${WORK_DIR}/s2i-run.log"
set -e
exit 1

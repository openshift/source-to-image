#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

STI_ROOT=$(dirname "${BASH_SOURCE}")/..
KILLDONE=""
STI_PID=""

function time_now()
{
  echo $(date +%s000)
}

tmpdir=$(time_now) 
WORK_DIR="/tmp/${tmpdir}"
function cleanup()
{
    set +e
    rm -rf ${WORK_DIR}
    # use sigint so that sti post processing will remove docker container
    if [ -R "$KILLDONE" ]; then
	if [ -R "$STI_PID" ]; then
	    kill -2 "${STI_PID}"
	fi
    fi
    echo
    echo "Complete"
}

function check_result() {
  local result="$1"
  if [[ "$result" != "0" ]]; then
      info "TEST FAILED (${result})"
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

echo
echo 
echo

echo "working dir:  ${WORK_DIR}"
mkdir -p ${WORK_DIR}
pushd ${WORK_DIR}

test_debug "cloning source into working dir"

git clone git://github.com/bparees/openshift-jee-sample &> "${WORK_DIR}/s2i-git-clone.log"
check_result $? "${WORK_DIR}/s2i-git-clone.log"

test_debug "s2i build with relative path without file://"

s2i build openshift-jee-sample openshift/wildfly-8-centos test-jee-app &> "${WORK_DIR}/s2i-rel-noproto.log"
check_result $? "${WORK_DIR}/s2i-rel-noproto.log"

test_debug "s2i build with relative path with file://"

s2i build "file://./openshift-jee-sample" openshift/wildfly-8-centos test-jee-app &> "${WORK_DIR}/s2i-rel-proto.log"
check_result $? "${WORK_DIR}/s2i-rel-proto.log"

popd

test_debug "s2i build with absolute path with file://"

s2i build "file://${WORK_DIR}/openshift-jee-sample" openshift/wildfly-8-centos test-jee-app &> "${WORK_DIR}/s2i-abs-proto.log"
check_result $? "${WORK_DIR}/s2i-abs-proto.log"

test_debug "s2i build with absolute path without file://"

s2i build "${WORK_DIR}/openshift-jee-sample" openshift/wildfly-8-centos test-jee-app &> "${WORK_DIR}/s2i-abs-noproto.log"
check_result $? "${WORK_DIR}/s2i-abs-noproto.log"

test_debug "s2i build with a no proto specified  ssh url"

s2i build git@github.com:bparees/openshift-jee-sample openshift/wildfly-8-centos test-jee-app &> "${WORK_DIR}/s2i-ssh-noproto.log"
check_result $? "${WORK_DIR}/s2i-ssh-noproto.log"

test_debug "s2i usage"

s2i usage openshift/ruby-20-centos7 &> "${WORK_DIR}/s2i-usage.log"
check_result $? "${WORK_DIR}/s2i-usage.log"

test_debug "s2i build with --run==true option"

s2i build git://github.com/bparees/openshift-jee-sample openshift/wildfly-8-centos test-jee-app --run=true &> "${WORK_DIR}/sti-run.log" &
STI_PID=$!
TIME_SEC=1000
TIME_MIN=$((60 * $TIME_SEC))
max_wait=10*TIME_MIN
echo "waiting up to ${max_wait}"
expire=$(($(time_now) + $max_wait))

set +e
while [[ $(time_now) -lt $expire ]]; do
    grep  "as a result of the --run=true option" "${WORK_DIR}/sti-run.log"
    if [ $? -eq 0 ]; then
      echo "[INFO] Success running command sti --run=true"
      
      # use sigint so that sti post processing will remove docker container
      kill -2 "${STI_PID}"
      KILLDONE="killed"
      sleep 30
      docker ps -a | grep test-jee-app

      if [ $? -eq 1 ]; then
	  echo "[INFO] Success terminating associated docker container"
	  exit 0
      else
	  echo "[INFO] Associated docker container still found, review docker ps -a output above, and here is the dump of ${WORK_DIR}/sti-run.log"
	  cat "${WORK_DIR}/sti-run.log"
	  exit 1
      fi
    fi
    sleep 1
done

echo "[INFO] Problem with sti --run=true, dumping ${WORK_DIR}/sti-run.log"
cat "${WORK_DIR}/sti-run.log"
set -e
exit 1

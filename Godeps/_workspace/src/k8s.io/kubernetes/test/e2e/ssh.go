/*
Copyright 2015 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"fmt"
	"strings"

	"k8s.io/kubernetes/test/e2e/framework"

	. "github.com/onsi/ginkgo"
)

var _ = framework.KubeDescribe("SSH", func() {

	f := framework.NewDefaultFramework("ssh")

	BeforeEach(func() {
		// When adding more providers here, also implement their functionality in util.go's framework.GetSigner(...).
		framework.SkipUnlessProviderIs(framework.ProvidersWithSSH...)
	})

	It("should SSH to all nodes and run commands", func() {
		// Get all nodes' external IPs.
		By("Getting all nodes' SSH-able IP addresses")
		hosts, err := framework.NodeSSHHosts(f.Client)
		if err != nil {
			framework.Failf("Error getting node hostnames: %v", err)
		}

		testCases := []struct {
			cmd            string
			checkStdout    bool
			expectedStdout string
			expectedStderr string
			expectedCode   int
			expectedError  error
		}{
			{`echo "Hello"`, true, "Hello", "", 0, nil},
			// Same as previous, but useful for test output diagnostics.
			{`echo "Hello from $(whoami)@$(hostname)"`, false, "", "", 0, nil},
			{`echo "foo" | grep "bar"`, true, "", "", 1, nil},
			{`echo "Out" && echo "Error" >&2 && exit 7`, true, "Out", "Error", 7, nil},
		}

		// Run commands on all nodes via SSH.
		for _, testCase := range testCases {
			By(fmt.Sprintf("SSH'ing to all nodes and running %s", testCase.cmd))
			for _, host := range hosts {
				result, err := framework.SSH(testCase.cmd, host, framework.TestContext.Provider)
				stdout, stderr := strings.TrimSpace(result.Stdout), strings.TrimSpace(result.Stderr)
				if err != testCase.expectedError {
					framework.Failf("Ran %s on %s, got error %v, expected %v", testCase.cmd, host, err, testCase.expectedError)
				}
				if testCase.checkStdout && stdout != testCase.expectedStdout {
					framework.Failf("Ran %s on %s, got stdout '%s', expected '%s'", testCase.cmd, host, stdout, testCase.expectedStdout)
				}
				if stderr != testCase.expectedStderr {
					framework.Failf("Ran %s on %s, got stderr '%s', expected '%s'", testCase.cmd, host, stderr, testCase.expectedStderr)
				}
				if result.Code != testCase.expectedCode {
					framework.Failf("Ran %s on %s, got exit code %d, expected %d", testCase.cmd, host, result.Code, testCase.expectedCode)
				}
				// Show stdout, stderr for logging purposes.
				if len(stdout) > 0 {
					framework.Logf("Got stdout from %s: %s", host, strings.TrimSpace(stdout))
				}
				if len(stderr) > 0 {
					framework.Logf("Got stderr from %s: %s", host, strings.TrimSpace(stderr))
				}
			}
		}

		// Quickly test that SSH itself errors correctly.
		By("SSH'ing to a nonexistent host")
		if _, err = framework.SSH(`echo "hello"`, "i.do.not.exist", framework.TestContext.Provider); err == nil {
			framework.Failf("Expected error trying to SSH to nonexistent host.")
		}
	})
})

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

package app

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"k8s.io/kubernetes/cmd/kube-proxy/app/options"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/util/iptables"
)

type fakeNodeInterface struct {
	node api.Node
}

func (fake *fakeNodeInterface) Get(hostname string) (*api.Node, error) {
	return &fake.node, nil
}

type fakeIptablesVersioner struct {
	version string // what to return
	err     error  // what to return
}

func (fake *fakeIptablesVersioner) GetVersion() (string, error) {
	return fake.version, fake.err
}

type fakeKernelCompatTester struct {
	ok bool
}

func (fake *fakeKernelCompatTester) IsCompatible() error {
	if !fake.ok {
		return fmt.Errorf("error")
	}
	return nil
}

func Test_getProxyMode(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-Linux")
	}
	var cases = []struct {
		flag            string
		annotationKey   string
		annotationVal   string
		iptablesVersion string
		kernelCompat    bool
		iptablesError   error
		expected        string
	}{
		{ // flag says userspace
			flag:     "userspace",
			expected: proxyModeUserspace,
		},
		{ // flag says iptables, error detecting version
			flag:          "iptables",
			iptablesError: fmt.Errorf("oops!"),
			expected:      proxyModeUserspace,
		},
		{ // flag says iptables, version too low
			flag:            "iptables",
			iptablesVersion: "0.0.0",
			expected:        proxyModeUserspace,
		},
		{ // flag says iptables, version ok, kernel not compatible
			flag:            "iptables",
			iptablesVersion: iptables.MinCheckVersion,
			kernelCompat:    false,
			expected:        proxyModeUserspace,
		},
		{ // flag says iptables, version ok, kernel is compatible
			flag:            "iptables",
			iptablesVersion: iptables.MinCheckVersion,
			kernelCompat:    true,
			expected:        proxyModeIptables,
		},
		{ // detect, error
			flag:          "",
			iptablesError: fmt.Errorf("oops!"),
			expected:      proxyModeUserspace,
		},
		{ // detect, version too low
			flag:            "",
			iptablesVersion: "0.0.0",
			expected:        proxyModeUserspace,
		},
		{ // detect, version ok, kernel not compatible
			flag:            "",
			iptablesVersion: iptables.MinCheckVersion,
			kernelCompat:    false,
			expected:        proxyModeUserspace,
		},
		{ // detect, version ok, kernel is compatible
			flag:            "",
			iptablesVersion: iptables.MinCheckVersion,
			kernelCompat:    true,
			expected:        proxyModeIptables,
		},
		{ // annotation says userspace
			flag:          "",
			annotationKey: "net.experimental.kubernetes.io/proxy-mode",
			annotationVal: "userspace",
			expected:      proxyModeUserspace,
		},
		{ // annotation says iptables, error detecting
			flag:          "",
			annotationKey: "net.experimental.kubernetes.io/proxy-mode",
			annotationVal: "iptables",
			iptablesError: fmt.Errorf("oops!"),
			expected:      proxyModeUserspace,
		},
		{ // annotation says iptables, version too low
			flag:            "",
			annotationKey:   "net.experimental.kubernetes.io/proxy-mode",
			annotationVal:   "iptables",
			iptablesVersion: "0.0.0",
			expected:        proxyModeUserspace,
		},
		{ // annotation says iptables, version ok, kernel not compatible
			flag:            "",
			annotationKey:   "net.experimental.kubernetes.io/proxy-mode",
			annotationVal:   "iptables",
			iptablesVersion: iptables.MinCheckVersion,
			kernelCompat:    false,
			expected:        proxyModeUserspace,
		},
		{ // annotation says iptables, version ok, kernel is compatible
			flag:            "",
			annotationKey:   "net.experimental.kubernetes.io/proxy-mode",
			annotationVal:   "iptables",
			iptablesVersion: iptables.MinCheckVersion,
			kernelCompat:    true,
			expected:        proxyModeIptables,
		},
		{ // annotation says something else, version ok
			flag:            "",
			annotationKey:   "net.experimental.kubernetes.io/proxy-mode",
			annotationVal:   "other",
			iptablesVersion: iptables.MinCheckVersion,
			kernelCompat:    true,
			expected:        proxyModeIptables,
		},
		{ // annotation says nothing, version ok
			flag:            "",
			annotationKey:   "net.experimental.kubernetes.io/proxy-mode",
			annotationVal:   "",
			iptablesVersion: iptables.MinCheckVersion,
			kernelCompat:    true,
			expected:        proxyModeIptables,
		},
		{ // annotation says userspace
			flag:          "",
			annotationKey: "net.beta.kubernetes.io/proxy-mode",
			annotationVal: "userspace",
			expected:      proxyModeUserspace,
		},
		{ // annotation says iptables, error detecting
			flag:          "",
			annotationKey: "net.beta.kubernetes.io/proxy-mode",
			annotationVal: "iptables",
			iptablesError: fmt.Errorf("oops!"),
			expected:      proxyModeUserspace,
		},
		{ // annotation says iptables, version too low
			flag:            "",
			annotationKey:   "net.beta.kubernetes.io/proxy-mode",
			annotationVal:   "iptables",
			iptablesVersion: "0.0.0",
			expected:        proxyModeUserspace,
		},
		{ // annotation says iptables, version ok, kernel not compatible
			flag:            "",
			annotationKey:   "net.beta.kubernetes.io/proxy-mode",
			annotationVal:   "iptables",
			iptablesVersion: iptables.MinCheckVersion,
			kernelCompat:    false,
			expected:        proxyModeUserspace,
		},
		{ // annotation says iptables, version ok, kernel is compatible
			flag:            "",
			annotationKey:   "net.beta.kubernetes.io/proxy-mode",
			annotationVal:   "iptables",
			iptablesVersion: iptables.MinCheckVersion,
			kernelCompat:    true,
			expected:        proxyModeIptables,
		},
		{ // annotation says something else, version ok
			flag:            "",
			annotationKey:   "net.beta.kubernetes.io/proxy-mode",
			annotationVal:   "other",
			iptablesVersion: iptables.MinCheckVersion,
			kernelCompat:    true,
			expected:        proxyModeIptables,
		},
		{ // annotation says nothing, version ok
			flag:            "",
			annotationKey:   "net.beta.kubernetes.io/proxy-mode",
			annotationVal:   "",
			iptablesVersion: iptables.MinCheckVersion,
			kernelCompat:    true,
			expected:        proxyModeIptables,
		},
		{ // flag says userspace, annotation disagrees
			flag:            "userspace",
			annotationKey:   "net.experimental.kubernetes.io/proxy-mode",
			annotationVal:   "iptables",
			iptablesVersion: iptables.MinCheckVersion,
			expected:        proxyModeUserspace,
		},
		{ // flag says iptables, annotation disagrees
			flag:            "iptables",
			annotationKey:   "net.experimental.kubernetes.io/proxy-mode",
			annotationVal:   "userspace",
			iptablesVersion: iptables.MinCheckVersion,
			kernelCompat:    true,
			expected:        proxyModeIptables,
		},
		{ // flag says userspace, annotation disagrees
			flag:            "userspace",
			annotationKey:   "net.beta.kubernetes.io/proxy-mode",
			annotationVal:   "iptables",
			iptablesVersion: iptables.MinCheckVersion,
			expected:        proxyModeUserspace,
		},
		{ // flag says iptables, annotation disagrees
			flag:            "iptables",
			annotationKey:   "net.beta.kubernetes.io/proxy-mode",
			annotationVal:   "userspace",
			iptablesVersion: iptables.MinCheckVersion,
			kernelCompat:    true,
			expected:        proxyModeIptables,
		},
	}
	for i, c := range cases {
		getter := &fakeNodeInterface{}
		getter.node.Annotations = map[string]string{c.annotationKey: c.annotationVal}
		versioner := &fakeIptablesVersioner{c.iptablesVersion, c.iptablesError}
		kcompater := &fakeKernelCompatTester{c.kernelCompat}
		r := getProxyMode(c.flag, getter, "host", versioner, kcompater)
		if r != c.expected {
			t.Errorf("Case[%d] Expected %q, got %q", i, c.expected, r)
		}
	}
}

// This test verifies that Proxy Server does not crash that means
// Config and iptinterface are not nil when CleanupAndExit is true.
// To avoid proxy crash: https://github.com/kubernetes/kubernetes/pull/14736
func TestProxyServerWithCleanupAndExit(t *testing.T) {

	// creates default config
	config := options.NewProxyConfig()

	// sets CleanupAndExit manually
	config.CleanupAndExit = true

	// creates new proxy server
	proxyserver, err := NewProxyServerDefault(config)

	// verifies that nothing is nill except error
	assert.Nil(t, err)
	assert.NotNil(t, proxyserver)
	assert.NotNil(t, proxyserver.Config)
	assert.NotNil(t, proxyserver.IptInterface)
}

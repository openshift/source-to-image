package test

import (
	"testing"

	"github.com/openshift/source-to-image/pkg/cmd/cli/cmd"
)

func TestGenerate_canonizeBuilderImageArg(t *testing.T) {
	assertCanonize := func(t *testing.T, input string, expected string) {
		got := cmd.CanonizeBuilderImageArg(input)
		if got != expected {
			t.Fail()
		}
	}

	assertCanonize(t, "docker://docker.io/centos/nodejs-10-centos7", "docker://docker.io/centos/nodejs-10-centos7")
	assertCanonize(t, "docker.io/centos/nodejs-10-centos7", "docker://docker.io/centos/nodejs-10-centos7")
}

package dockerfile

import (
	"bytes"
	"io/ioutil"
	"os"
	"regexp"
	"testing"

	"github.com/moby/buildkit/frontend/dockerfile/parser"

	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/build"
	"github.com/openshift/source-to-image/pkg/build/strategies"
)

// RunDockerfileTest execute the tests against a dockerfile, check for expected, not expected entries
// in generated dockerfile, and also checks for expected failures.
func RunDockerfileTest(t *testing.T, config *api.Config, expected []string, notExpected []string, expectedFiles []string, expectFailure bool) {
	b, _, err := strategies.Strategy(nil, config, build.Overrides{})
	if err != nil {
		t.Fatalf("Cannot create a new builder.")
	}
	resp, err := b.Build(config)
	if expectFailure {
		if err == nil || resp.Success {
			t.Errorf("The build succeeded when it should have failed. Success: %t, error: %v", resp.Success, err)
		}
		return
	}
	if err != nil {
		t.Fatalf("An error occurred during the build: %v", err)
	}
	if !resp.Success {
		t.Fatalf("The build failed when it should have succeeded.")
	}

	filebytes, err := ioutil.ReadFile(config.AsDockerfile)
	if err != nil {
		t.Fatalf("An error occurred reading the dockerfile: %v", err)
	}
	dockerfile := string(filebytes)

	buf := bytes.NewBuffer(filebytes)
	_, err = parser.Parse(buf)
	if err != nil {
		t.Fatalf("An error occurred parsing the dockerfile: %v\n%s", err, dockerfile)
	}

	for _, s := range expected {
		reg, err := regexp.Compile(s)
		if err != nil {
			t.Fatalf("failed to compile regex %q: %v", s, err)
		}
		if !reg.MatchString(dockerfile) {
			t.Fatalf("Expected dockerfile to contain %s, it did not: \n%s", s, dockerfile)
		}
	}
	for _, s := range notExpected {
		reg, err := regexp.Compile(s)
		if err != nil {
			t.Fatalf("failed to compile regex %q: %v", s, err)
		}
		if reg.MatchString(dockerfile) {
			t.Fatalf("Expected dockerfile not to contain %s, it did: \n%s", s, dockerfile)
		}
	}
	for _, f := range expectedFiles {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			t.Fatalf("Did not find expected file %s, ", f)
		}
	}
}

// +build integration

package docker

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/scm/git"
	"github.com/openshift/source-to-image/test/integration/dockerfile"
)

// TestDockerfileWithBuilder exercises --with-builder flag by letting s2i generate a Dockerfile
// and later on building the image, with external builder.
func TestDockerfileWithBuilder(t *testing.T) {
	tempdir, err := ioutil.TempDir("", "s2i-withbuildertest-dir")
	if err != nil {
		t.Fatal(err)
	}
	// defer os.RemoveAll(tempdir)

	// making sure the Dockerfile is generate on temporary directory
	config := &api.Config{
		BuilderImage: "docker.io/centos/nodejs-8-centos7",
		WorkingDir:   tempdir,
		Source:       git.MustParse("https://github.com/otaviof/nodejs-ex"),
		Tag:          "test:tag",
		WithBuilder:  "docker",
	}

	// simple probe of dockerfile, it's already extensively tested
	expected := []string{"^FROM.*"}
	// expect to find generated Dockerfile with s2i extension
	expectedFiles := []string{filepath.Join(tempdir, "Dockerfile.s2i")}

	dockerfile.RunDockerfileTest(t, config, expected, nil, expectedFiles, false)
}

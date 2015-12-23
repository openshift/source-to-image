package layered

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/docker"
	"github.com/openshift/source-to-image/pkg/test"
)

type FakeExecutor struct{}

func (f *FakeExecutor) Execute(string, string, *api.Config) error {
	return nil
}

func newFakeLayered() *Layered {
	return &Layered{
		docker:  &docker.FakeDocker{},
		config:  &api.Config{},
		fs:      &test.FakeFileSystem{},
		tar:     &test.FakeTar{},
		scripts: &FakeExecutor{},
	}
}

func newFakeLayeredWithScripts(assemble, workDir string) *Layered {
	return &Layered{
		docker:  &docker.FakeDocker{},
		config:  &api.Config{WorkingDir: workDir},
		fs:      &test.FakeFileSystem{},
		tar:     &test.FakeTar{},
		scripts: &FakeExecutor{},
	}
}

func TestBuildOK(t *testing.T) {
	workDir, _ := ioutil.TempDir("", "sti")
	scriptDir := filepath.Join(workDir, api.UploadScripts)
	err := os.MkdirAll(scriptDir, 0700)
	assemble := filepath.Join(scriptDir, api.Assemble)
	file, err := os.Create(assemble)
	if err != nil {
		t.Errorf("Unexpected error returned: %v", err)
	}
	defer file.Close()
	defer os.RemoveAll(workDir)
	l := newFakeLayeredWithScripts(assemble, workDir)
	l.config.BuilderImage = "test/image"
	_, err = l.Build(l.config)
	if err != nil {
		t.Errorf("Unexpected error returned: %v", err)
	}
	if !l.config.LayeredBuild {
		t.Errorf("Expected LayeredBuild to be true!")
	}
	if m, _ := regexp.MatchString(`test/image-\d+`, l.config.BuilderImage); !m {
		t.Errorf("Expected BuilderImage test/image-withnumbers, but got %s ", l.config.BuilderImage)
	}
	// without config.Destination explicitly set, we should get /tmp/scripts for the scripts url
	// assuming the assemble script we created above is off the working dir
	if l.config.ScriptsURL != "image:///tmp/scripts" {
		t.Errorf("Expected ScriptsURL image:///tmp/scripts, but got %s", l.config.ScriptsURL)
	}
	if len(l.config.Destination) != 0 {
		t.Errorf("Unexpected Destination %s", l.config.Destination)
	}
}

func TestBuildNoScriptsProvided(t *testing.T) {
	l := newFakeLayered()
	l.config.BuilderImage = "test/image"
	_, err := l.Build(l.config)
	if err != nil {
		t.Errorf("Unexpected error returned: %v", err)
	}
	if !l.config.LayeredBuild {
		t.Errorf("Expected LayeredBuild to be true!")
	}
	if m, _ := regexp.MatchString(`test/image-\d+`, l.config.BuilderImage); !m {
		t.Errorf("Expected BuilderImage test/image-withnumbers, but got %s", l.config.BuilderImage)
	}
	if len(l.config.Destination) != 0 {
		t.Errorf("Unexpected Destination %s", l.config.Destination)
	}
}

func TestBuildErrorWriteDockerfile(t *testing.T) {
	l := newFakeLayered()
	l.fs.(*test.FakeFileSystem).WriteFileError = errors.New("WriteDockerfileError")
	_, err := l.Build(l.config)
	if err == nil || err.Error() != "WriteDockerfileError" {
		t.Errorf("An error was expected for WriteDockerfile, but got different: %v", err)
	}
}

func TestBuildErrorCreateTarFile(t *testing.T) {
	l := newFakeLayered()
	l.tar.(*test.FakeTar).CreateTarError = errors.New("CreateTarError")
	_, err := l.Build(l.config)
	if err == nil || err.Error() != "CreateTarError" {
		t.Error("An error was expected for CreateTar, but got different: %v", err)
	}
}

func TestBuildErrorOpenTarFile(t *testing.T) {
	l := newFakeLayered()
	l.fs.(*test.FakeFileSystem).OpenError = errors.New("OpenTarError")
	_, err := l.Build(l.config)
	if err == nil || err.Error() != "OpenTarError" {
		t.Errorf("An error was expected for OpenTarFile, but got different: %v", err)
	}
}

func TestBuildErrorBuildImage(t *testing.T) {
	l := newFakeLayered()
	l.docker.(*docker.FakeDocker).BuildImageError = errors.New("BuildImageError")
	_, err := l.Build(l.config)
	if err == nil || err.Error() != "BuildImageError" {
		t.Errorf("An error was expected for BuildImage, but got different: %v", err)
	}
}

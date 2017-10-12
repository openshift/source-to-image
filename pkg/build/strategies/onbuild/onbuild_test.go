package onbuild

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/docker/builder/dockerfile/parser"

	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/docker"
	"github.com/openshift/source-to-image/pkg/scm/git"
	"github.com/openshift/source-to-image/pkg/test"
	testfs "github.com/openshift/source-to-image/pkg/test/fs"
	"github.com/openshift/source-to-image/pkg/util/fs"
)

type fakeSourceHandler struct{}

func (*fakeSourceHandler) Prepare(r *api.Config) error {
	return nil
}

func (*fakeSourceHandler) Ignore(r *api.Config) error {
	return nil
}

func (*fakeSourceHandler) Download(r *api.Config) (*git.SourceInfo, error) {
	return &git.SourceInfo{}, nil
}

type fakeCleaner struct{}

func (*fakeCleaner) Cleanup(*api.Config) {}

func newFakeOnBuild() *OnBuild {
	return &OnBuild{
		docker:  &docker.FakeDocker{},
		git:     &test.FakeGit{},
		fs:      &testfs.FakeFileSystem{},
		tar:     &test.FakeTar{},
		source:  &fakeSourceHandler{},
		garbage: &fakeCleaner{},
	}
}

func checkDockerfile(fs *testfs.FakeFileSystem, t *testing.T) {
	if fs.WriteFileError != nil {
		t.Errorf("%v", fs.WriteFileError)
	}
	if filepath.ToSlash(fs.WriteFileName) != "upload/src/Dockerfile" {
		t.Errorf("Expected Dockerfile in 'upload/src/Dockerfile', got %q", fs.WriteFileName)
	}
	if !strings.Contains(fs.WriteFileContent, `ENTRYPOINT ["./run"]`) {
		t.Errorf("The Dockerfile does not set correct entrypoint, file content:\n%s", fs.WriteFileContent)
	}

	buf := bytes.NewBuffer([]byte(fs.WriteFileContent))
	d := &parser.Directive{}
	parser.SetEscapeToken(parser.DefaultEscapeToken, d)
	if _, err := parser.Parse(buf, d); err != nil {
		t.Errorf("cannot parse new Dockerfile: %v", err)
	}

}

func TestCreateDockerfile(t *testing.T) {
	fakeRequest := &api.Config{
		BuilderImage: "fake:onbuild",
		Environment: api.EnvironmentList{
			{Name: "FOO", Value: "BAR"},
			{Name: "TEST", Value: "A VALUE"},
		},
	}
	b := newFakeOnBuild()
	fakeFs := &testfs.FakeFileSystem{
		Files: []os.FileInfo{
			&fs.FileInfo{FileName: "config.ru", FileMode: 0600},
			&fs.FileInfo{FileName: "app.rb", FileMode: 0600},
			&fs.FileInfo{FileName: "run", FileMode: 0777},
		},
	}
	b.fs = fakeFs
	err := b.CreateDockerfile(fakeRequest)
	if err != nil {
		t.Errorf("%v", err)
	}
	checkDockerfile(fakeFs, t)
}

func TestCreateDockerfileWithAssemble(t *testing.T) {
	fakeRequest := &api.Config{
		BuilderImage: "fake:onbuild",
	}
	b := newFakeOnBuild()
	fakeFs := &testfs.FakeFileSystem{
		Files: []os.FileInfo{
			&fs.FileInfo{FileName: "config.ru", FileMode: 0600},
			&fs.FileInfo{FileName: "app.rb", FileMode: 0600},
			&fs.FileInfo{FileName: "run", FileMode: 0777},
			&fs.FileInfo{FileName: "assemble", FileMode: 0777},
		},
	}
	b.fs = fakeFs
	err := b.CreateDockerfile(fakeRequest)
	if err != nil {
		t.Errorf("%v", err)
	}
	checkDockerfile(fakeFs, t)
	if !strings.Contains(fakeFs.WriteFileContent, `RUN sh assemble`) {
		t.Errorf("The Dockerfile does not run assemble, file content:\n%s", fakeFs.WriteFileContent)
	}
}

func TestBuild(t *testing.T) {
	fakeRequest := &api.Config{
		BuilderImage: "fake:onbuild",
		Tag:          "fakeapp",
	}
	b := newFakeOnBuild()
	fakeFs := &testfs.FakeFileSystem{
		Files: []os.FileInfo{
			&fs.FileInfo{FileName: "config.ru", FileMode: 0600},
			&fs.FileInfo{FileName: "app.rb", FileMode: 0600},
			&fs.FileInfo{FileName: "run", FileMode: 0777},
		},
	}
	b.fs = fakeFs
	result, err := b.Build(fakeRequest)
	if err != nil {
		t.Errorf("%v", err)
	}
	if !result.Success {
		t.Errorf("Expected successful build, got: %v", result)
	}
	checkDockerfile(fakeFs, t)
	t.Logf("result: %v", result)
}

func TestBuildOnBuildBlocked(t *testing.T) {
	fakeRequest := &api.Config{
		BuilderImage: "fake:onbuild",
		Tag:          "fakeapp",
		BlockOnBuild: true,
	}
	b := newFakeOnBuild()
	fakeFs := &testfs.FakeFileSystem{
		Files: []os.FileInfo{
			&fs.FileInfo{FileName: "config.ru", FileMode: 0600},
			&fs.FileInfo{FileName: "app.rb", FileMode: 0600},
			&fs.FileInfo{FileName: "run", FileMode: 0777},
		},
	}
	b.fs = fakeFs
	_, err := b.Build(fakeRequest)
	if err == nil || !strings.Contains(err.Error(), "builder image uses ONBUILD instructions but ONBUILD is not allowed") {
		t.Errorf("expected error from onbuild due to blocked ONBUILD, got: %v", err)
	}
}

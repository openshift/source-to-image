package onbuild

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/golang/glog"
	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/build"
	"github.com/openshift/source-to-image/pkg/docker"
	"github.com/openshift/source-to-image/pkg/git"
	"github.com/openshift/source-to-image/pkg/tar"
	"github.com/openshift/source-to-image/pkg/util"
)

// OnBuild strategy executes the simple Docker build in case the image does not
// support STI scripts but has ONBUILD instructions recorded.
type OnBuild struct {
	docker  docker.Docker
	git     git.Git
	fs      util.FileSystem
	tar     tar.Tar
	source  build.Downloader
	garbage build.Cleaner
}

// New returns a new instance of OnBuild builder
func New(request *api.Request) (*OnBuild, error) {
	dockerHandler, err := docker.New(request.DockerSocket)
	if err != nil {
		return nil, err
	}
	b := &OnBuild{
		docker: dockerHandler,
		git:    git.New(),
		fs:     util.NewFileSystem(),
		tar:    tar.New(),
	}
	b.source = &git.Clone{b.git, b.fs}
	b.garbage = &build.DefaultCleaner{b.fs, b.docker}
	return b, nil
}

// SourceTar produces a tar archive containing application source and stream it
func (b *OnBuild) SourceTar(request *api.Request) (io.ReadCloser, error) {
	uploadDir := filepath.Join(request.WorkingDir, "upload", "src")
	tarFileName, err := b.tar.CreateTarFile(request.WorkingDir, uploadDir)
	if err != nil {
		return nil, err
	}
	return b.fs.Open(tarFileName)
}

// Build executes the ONBUILD kind of build
func (b *OnBuild) Build(request *api.Request) (*api.Result, error) {
	glog.V(2).Info("Preparing the source code for build")
	if err := b.Prepare(request); err != nil {
		return nil, err
	}

	glog.V(2).Info("Creating application Dockerfile")
	if err := b.CreateDockerfile(request); err != nil {
		return nil, err
	}

	glog.V(2).Info("Creating application source code image")
	tarStream, err := b.SourceTar(request)
	if err != nil {
		return nil, err
	}
	defer tarStream.Close()

	opts := docker.BuildImageOptions{
		Name:   request.Tag,
		Stdin:  tarStream,
		Stdout: os.Stdout,
	}

	glog.V(2).Info("Building the application source")
	if err := b.docker.BuildImage(opts); err != nil {
		return nil, err
	}

	glog.V(2).Info("Cleaning up temporary containers")
	b.garbage.Cleanup(request)

	return &api.Result{
		Success:    true,
		WorkingDir: request.WorkingDir,
		ImageID:    opts.Name,
	}, nil
}

// CreateDockerfile creates the ONBUILD Dockerfile
func (b *OnBuild) CreateDockerfile(request *api.Request) error {
	buffer := bytes.Buffer{}
	uploadDir := filepath.Join(request.WorkingDir, "upload", "src")
	buffer.WriteString(fmt.Sprintf("FROM %s\n", request.BaseImage))
	entrypoint, err := GuessEntrypoint(uploadDir)
	if err != nil {
		return err
	}
	buffer.WriteString(fmt.Sprintf(`CMD ["./%s"]`+"\n", entrypoint))
	return b.fs.WriteFile(filepath.Join(uploadDir, "Dockerfile"), buffer.Bytes())
}

// Prepare prepares the source code and the Docker image image for the build
func (b *OnBuild) Prepare(request *api.Request) error {
	// Pull the Docker image if it does not exists in local Docker
	if request.ForcePull {
		b.docker.PullImage(request.BaseImage)
	} else {
		b.docker.CheckAndPull(request.BaseImage)
	}

	tempDir, err := b.fs.CreateWorkingDirectory()
	if err != nil {
		return err
	}
	request.WorkingDir = tempDir

	return b.source.Download(request)
}

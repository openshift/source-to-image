// Package run supports running images produced by S2I. It is used by the
// --run=true command line option.
package run

import (
	"io"

	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/docker"
	"github.com/openshift/source-to-image/pkg/errors"
	utilglog "github.com/openshift/source-to-image/pkg/util/glog"
)

var glog = utilglog.StderrLog

// A DockerRunner allows running a Docker image as a new container, streaming
// stdout and stderr with glog.
type DockerRunner struct {
	ContainerClient docker.Docker
}

// New creates a DockerRunner for executing the methods associated with running
// the produced image in a docker container for verification purposes.
func New(config *api.Config) (*DockerRunner, error) {
	client, err := docker.New(config.DockerConfig, config.PullAuthentication)
	if err != nil {
		glog.Errorf("Failed to connect to Docker daemon: %v", err)
		return nil, err
	}
	return &DockerRunner{client}, nil
}

// Run invokes the Docker API to run the image defined in config as a new
// container. The container's stdout and stderr will be logged with glog.
func (b *DockerRunner) Run(config *api.Config) error {
	glog.V(4).Infof("Attempting to run image %s \n", config.Tag)

	outReader, outWriter := io.Pipe()
	defer outReader.Close()
	defer outWriter.Close()
	errReader, errWriter := io.Pipe()
	defer errReader.Close()
	defer errWriter.Close()

	opts := docker.RunContainerOptions{
		Image:        config.Tag,
		Stdout:       outWriter,
		Stderr:       errWriter,
		TargetImage:  true,
		CGroupLimits: config.CGroupLimits,
		CapDrop:      config.DropCapabilities,
	}

	//NOTE, we've seen some Golang level deadlock issues with the streaming of cmd output to
	// glog, but part of the deadlock seems to have occurred when stdout was "silent"
	// and produced no data, such as when we would do a git clone with the --quiet option.
	// We have not seen the hang when the Cmd produces output to stdout.

	go docker.StreamContainerIO(errReader, nil, glog.Error)
	go docker.StreamContainerIO(outReader, nil, glog.Info)

	err := b.ContainerClient.RunContainer(opts)
	// If we get a ContainerError, the original message reports the
	// container name. The container is temporary and its name is
	// meaningless, therefore we make the error message more helpful by
	// replacing the container name with the image tag.
	if e, ok := err.(errors.ContainerError); ok {
		return errors.NewContainerError(config.Tag, e.ErrorCode, e.Output)
	}
	return err
}

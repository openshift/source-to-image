package test

import (
	dockertypes "github.com/docker/engine-api/types"
	"golang.org/x/net/context"
	"io"
)

// FakeDockerClient provides a Fake client for Docker testing, but for our direct access to the engine-api client;
// we leverage the FakeDockerClient defined in k8s when we leverage the k8s layer
type FakeDockerClient struct {
	CopyToContainerID      string
	CopyToContainerPath    string
	CopyToContainerContent io.Reader
	CopyToContainerOpts    dockertypes.CopyToContainerOptions
	CopyToContainerErr     error

	WaitContainerID     string
	WaitContainerResult int
	WaitContainerErr    error

	ContainerCommitID       string
	ContainerCommitOptions  dockertypes.ContainerCommitOptions
	ContainerCommitResponse dockertypes.ContainerCommitResponse
	ContainerCommitErr      error

	BuildContext       io.Reader
	BuildImageOpts     dockertypes.ImageBuildOptions
	BuildImageResponse dockertypes.ImageBuildResponse
	BuildImageErr      error
}

func (d *FakeDockerClient) Ping() error {
	return nil
}

func (d *FakeDockerClient) CopyToContainer(ctx context.Context, container, path string, content io.Reader, opts dockertypes.CopyToContainerOptions) error {
	return nil
}

func (d *FakeDockerClient) CopyFromContainer(ctx context.Context, container, srcPath string) (io.ReadCloser, dockertypes.ContainerPathStat, error) {
	return nil, dockertypes.ContainerPathStat{}, nil
}

func (d *FakeDockerClient) ContainerWait(ctx context.Context, containerID string) (int, error) {
	d.WaitContainerID = containerID
	return d.WaitContainerResult, d.WaitContainerErr
}

func (d *FakeDockerClient) ContainerCommit(ctx context.Context, container string, options dockertypes.ContainerCommitOptions) (dockertypes.ContainerCommitResponse, error) {
	d.ContainerCommitID = container
	d.ContainerCommitOptions = options
	return d.ContainerCommitResponse, d.ContainerCommitErr
}

func (d *FakeDockerClient) ImageBuild(ctx context.Context, buildContext io.Reader, options dockertypes.ImageBuildOptions) (dockertypes.ImageBuildResponse, error) {
	d.BuildImageOpts = options
	d.BuildContext = buildContext
	return d.BuildImageResponse, d.BuildImageErr
}

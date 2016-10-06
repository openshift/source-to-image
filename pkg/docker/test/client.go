package test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"time"

	dockertypes "github.com/docker/engine-api/types"
	dockercontainer "github.com/docker/engine-api/types/container"
	dockernetwork "github.com/docker/engine-api/types/network"
	"golang.org/x/net/context"
)

type FakeDockerAddr struct {
}

func (a FakeDockerAddr) Network() string {
	return ""
}

func (a FakeDockerAddr) String() string {
	return ""
}

type FakeDockerConn struct {
}

func (c FakeDockerConn) Read(b []byte) (n int, err error) {
	return 0, nil
}

func (c FakeDockerConn) Write(b []byte) (n int, err error) {
	return 0, nil
}

func (c FakeDockerConn) Close() error {
	return nil
}

func (c FakeDockerConn) LocalAddr() net.Addr {
	return FakeDockerAddr{}
}

func (c FakeDockerConn) RemoteAddr() net.Addr {
	return FakeDockerAddr{}
}

func (c FakeDockerConn) SetDeadline(t time.Time) error {
	return nil
}

func (c FakeDockerConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c FakeDockerConn) SetWriteDeadline(t time.Time) error {
	return nil
}

// FakeDockerClient provides a Fake client for Docker testing
type FakeDockerClient struct {
	CopyToContainerID      string
	CopyToContainerPath    string
	CopyToContainerContent io.Reader

	CopyFromContainerID   string
	CopyFromContainerPath string
	CopyFromContainerErr  error

	WaitContainerID     string
	WaitContainerResult int
	WaitContainerErr    error

	ContainerCommitID       string
	ContainerCommitOptions  dockertypes.ContainerCommitOptions
	ContainerCommitResponse dockertypes.ContainerCommitResponse
	ContainerCommitErr      error

	BuildImageOpts dockertypes.ImageBuildOptions
	BuildImageErr  error
	Images         map[string]dockertypes.ImageInspect

	Containers map[string]dockercontainer.Config

	PullFail error

	Calls []string
}

func NewFakeDockerClient() *FakeDockerClient {
	return &FakeDockerClient{
		Images:     make(map[string]dockertypes.ImageInspect),
		Containers: make(map[string]dockercontainer.Config),
		Calls:      make([]string, 0),
	}
}

func (d *FakeDockerClient) ImageInspectWithRaw(ctx context.Context, imageID string, getSize bool) (dockertypes.ImageInspect, []byte, error) {
	d.Calls = append(d.Calls, "inspect_image")

	if _, exists := d.Images[imageID]; exists {
		return d.Images[imageID], nil, nil
	}
	return dockertypes.ImageInspect{}, nil, fmt.Errorf("No such image: %q", imageID)
}

func (d *FakeDockerClient) CopyToContainer(ctx context.Context, container, path string, content io.Reader, opts dockertypes.CopyToContainerOptions) error {
	d.CopyToContainerID = container
	d.CopyToContainerPath = path
	d.CopyToContainerContent = content
	return nil
}

func (d *FakeDockerClient) CopyFromContainer(ctx context.Context, container, srcPath string) (io.ReadCloser, dockertypes.ContainerPathStat, error) {
	d.CopyFromContainerID = container
	d.CopyFromContainerPath = srcPath
	return ioutil.NopCloser(bytes.NewReader([]byte(""))), dockertypes.ContainerPathStat{}, d.CopyFromContainerErr
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

func (d *FakeDockerClient) ContainerAttach(ctx context.Context, container string, options dockertypes.ContainerAttachOptions) (dockertypes.HijackedResponse, error) {
	d.Calls = append(d.Calls, "attach")
	return dockertypes.HijackedResponse{Conn: FakeDockerConn{}}, nil
}

func (d *FakeDockerClient) ImageBuild(ctx context.Context, buildContext io.Reader, options dockertypes.ImageBuildOptions) (dockertypes.ImageBuildResponse, error) {
	d.BuildImageOpts = options
	return dockertypes.ImageBuildResponse{
		Body: ioutil.NopCloser(bytes.NewReader([]byte(""))),
	}, d.BuildImageErr
}

func (d *FakeDockerClient) ContainerCreate(ctx context.Context, config *dockercontainer.Config, hostConfig *dockercontainer.HostConfig, networkingConfig *dockernetwork.NetworkingConfig, containerName string) (dockertypes.ContainerCreateResponse, error) {
	d.Calls = append(d.Calls, "create")

	d.Containers[containerName] = *config
	return dockertypes.ContainerCreateResponse{}, nil
}

func (d *FakeDockerClient) ContainerInspect(ctx context.Context, containerID string) (dockertypes.ContainerJSON, error) {
	d.Calls = append(d.Calls, "inspect_container")
	return dockertypes.ContainerJSON{}, nil
}

func (d *FakeDockerClient) ContainerRemove(ctx context.Context, containerID string, options dockertypes.ContainerRemoveOptions) error {
	d.Calls = append(d.Calls, "remove")

	if _, exists := d.Containers[containerID]; exists {
		delete(d.Containers, containerID)
		return nil
	}
	return errors.New("container does not exist")
}

func (d *FakeDockerClient) ContainerStart(ctx context.Context, containerID string) error {
	d.Calls = append(d.Calls, "start")
	return nil
}

func (d *FakeDockerClient) ImagePull(ctx context.Context, ref string, options dockertypes.ImagePullOptions) (io.ReadCloser, error) {
	d.Calls = append(d.Calls, "pull")

	if d.PullFail != nil {
		return nil, d.PullFail
	}

	return ioutil.NopCloser(bytes.NewReader([]byte{})), nil
}

func (d *FakeDockerClient) ImageRemove(ctx context.Context, imageID string, options dockertypes.ImageRemoveOptions) ([]dockertypes.ImageDelete, error) {
	d.Calls = append(d.Calls, "remove_image")

	if _, exists := d.Images[imageID]; exists {
		delete(d.Images, imageID)
		return []dockertypes.ImageDelete{}, nil
	}
	return []dockertypes.ImageDelete{}, errors.New("image does not exist")
}

func (d *FakeDockerClient) ServerVersion(ctx context.Context) (dockertypes.Version, error) {
	return dockertypes.Version{}, nil
}

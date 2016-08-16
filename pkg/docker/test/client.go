package test

import (
	"bytes"
	dockertypes "github.com/docker/engine-api/types"
	"golang.org/x/net/context"
	"io"
	"io/ioutil"
	"net"
	"time"
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

// FakeDockerClient provides a Fake client for Docker testing, but for our direct access to the engine-api client;
// we leverage the FakeDockerClient defined in k8s when we leverage the k8s layer
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
}

func (d *FakeDockerClient) Ping() error {
	return nil
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
	return dockertypes.HijackedResponse{Conn: FakeDockerConn{}}, nil
}

func (d *FakeDockerClient) ImageBuild(ctx context.Context, buildContext io.Reader, options dockertypes.ImageBuildOptions) (dockertypes.ImageBuildResponse, error) {
	d.BuildImageOpts = options
	return dockertypes.ImageBuildResponse{
		Body: ioutil.NopCloser(bytes.NewReader([]byte(""))),
	}, d.BuildImageErr
}

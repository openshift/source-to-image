package test

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"iter"
	"net"
	"time"

	mobyContainer "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/jsonstream"
	mobyClient "github.com/moby/moby/client"
	"golang.org/x/net/context"
)

// FakeConn fakes a net.Conn
type FakeConn struct {
}

// Read reads bytes
func (c FakeConn) Read(b []byte) (n int, err error) {
	return 0, nil
}

// Write writes bytes
func (c FakeConn) Write(b []byte) (n int, err error) {
	return 0, nil
}

// Close closes the connection
func (c FakeConn) Close() error {
	return nil
}

// LocalAddr returns the local address
func (c FakeConn) LocalAddr() net.Addr {
	ip, _ := net.ResolveIPAddr("ip4", "127.0.0.1")
	return ip
}

// RemoteAddr returns the remote address
func (c FakeConn) RemoteAddr() net.Addr {
	ip, _ := net.ResolveIPAddr("ip4", "127.0.0.1")
	return ip
}

// SetDeadline sets the deadline
func (c FakeConn) SetDeadline(t time.Time) error {
	return nil
}

// SetReadDeadline sets the read deadline
func (c FakeConn) SetReadDeadline(t time.Time) error {
	return nil
}

// SetWriteDeadline sets the write deadline
func (c FakeConn) SetWriteDeadline(t time.Time) error {
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

	WaitContainerID             string
	WaitContainerResult         int
	WaitContainerErr            error
	WaitContainerErrInspectJSON mobyClient.ContainerInspectResult

	ContainerCommitID       string
	ContainerCommitOptions  mobyClient.ContainerCommitOptions
	ContainerCommitResponse mobyClient.ContainerCommitResult
	ContainerCommitErr      error

	BuildImageOpts mobyClient.ImageBuildOptions
	BuildImageErr  error
	Images         map[string]mobyClient.ImageInspectResult

	Containers map[string]mobyContainer.Config

	PullFail error

	Calls []string
}

// NewFakeDockerClient returns a new FakeDockerClient
func NewFakeDockerClient() *FakeDockerClient {
	return &FakeDockerClient{
		Images:     make(map[string]mobyClient.ImageInspectResult),
		Containers: make(map[string]mobyContainer.Config),
		Calls:      make([]string, 0),
	}
}

// ImageInspectWithRaw returns the image information and its raw representation.
func (d *FakeDockerClient) ImageInspect(ctx context.Context, imageID string, _ ...mobyClient.ImageInspectOption) (mobyClient.ImageInspectResult, error) {
	d.Calls = append(d.Calls, "inspect_image")

	if _, exists := d.Images[imageID]; exists {
		return d.Images[imageID], nil
	}
	return mobyClient.ImageInspectResult{}, fmt.Errorf("No such image: %q", imageID)
}

// CopyToContainer copies content into the container filesystem.
func (d *FakeDockerClient) CopyToContainer(ctx context.Context, container string, options mobyClient.CopyToContainerOptions) (mobyClient.CopyToContainerResult, error) {
	d.CopyToContainerID = container
	d.CopyToContainerPath = options.DestinationPath
	d.CopyToContainerContent = options.Content
	return mobyClient.CopyToContainerResult{}, nil
}

// CopyFromContainer gets the content from the container and returns it as a Reader
// to manipulate it in the host. It's up to the caller to close the reader.
func (d *FakeDockerClient) CopyFromContainer(ctx context.Context, container string, options mobyClient.CopyFromContainerOptions) (mobyClient.CopyFromContainerResult, error) {
	d.CopyFromContainerID = container
	d.CopyFromContainerPath = options.SourcePath
	return mobyClient.CopyFromContainerResult{Content: io.NopCloser(bytes.NewReader([]byte("")))}, d.CopyFromContainerErr
}

// ContainerWait pauses execution until a container exits.
func (d *FakeDockerClient) ContainerWait(ctx context.Context, containerID string, options mobyClient.ContainerWaitOptions) mobyClient.ContainerWaitResult {
	d.WaitContainerID = containerID
	resultC := make(chan mobyContainer.WaitResponse)
	errC := make(chan error, 1)

	go func() {
		if d.WaitContainerErr != nil {
			errC <- d.WaitContainerErr
			return
		}

		resultC <- mobyContainer.WaitResponse{StatusCode: int64(d.WaitContainerResult)}
	}()

	return mobyClient.ContainerWaitResult{Result: resultC, Error: errC}
}

// ContainerCommit applies changes into a container and creates a new tagged image.
func (d *FakeDockerClient) ContainerCommit(ctx context.Context, container string, options mobyClient.ContainerCommitOptions) (mobyClient.ContainerCommitResult, error) {
	d.ContainerCommitID = container
	d.ContainerCommitOptions = options
	return d.ContainerCommitResponse, d.ContainerCommitErr
}

// ContainerAttach attaches a connection to a container in the server.
func (d *FakeDockerClient) ContainerAttach(ctx context.Context, container string, options mobyClient.ContainerAttachOptions) (mobyClient.ContainerAttachResult, error) {
	d.Calls = append(d.Calls, "attach")
	return mobyClient.ContainerAttachResult{HijackedResponse: mobyClient.HijackedResponse{Conn: FakeConn{}, Reader: bufio.NewReader(&bytes.Buffer{})}}, nil
}

// ImageBuild sends request to the daemon to build images.
func (d *FakeDockerClient) ImageBuild(ctx context.Context, context io.Reader, options mobyClient.ImageBuildOptions) (mobyClient.ImageBuildResult, error) {
	d.BuildImageOpts = options
	return mobyClient.ImageBuildResult{
		Body: io.NopCloser(bytes.NewReader([]byte(""))),
	}, d.BuildImageErr
}

// ContainerCreate creates a new container based in the given configuration.
func (d *FakeDockerClient) ContainerCreate(ctx context.Context, options mobyClient.ContainerCreateOptions) (mobyClient.ContainerCreateResult, error) {
	d.Calls = append(d.Calls, "create")

	d.Containers[options.Name] = *options.Config
	return mobyClient.ContainerCreateResult{}, nil
}

// ContainerInspect returns the container information.
func (d *FakeDockerClient) ContainerInspect(ctx context.Context, container string, options mobyClient.ContainerInspectOptions) (mobyClient.ContainerInspectResult, error) {
	d.Calls = append(d.Calls, "inspect_container")
	return d.WaitContainerErrInspectJSON, nil
}

// ContainerRemove kills and removes a container from the docker host.
func (d *FakeDockerClient) ContainerRemove(ctx context.Context, containerID string, options mobyClient.ContainerRemoveOptions) (mobyClient.ContainerRemoveResult, error) {
	d.Calls = append(d.Calls, "remove")

	if _, exists := d.Containers[containerID]; exists {
		delete(d.Containers, containerID)
		return mobyClient.ContainerRemoveResult{}, nil
	}
	return mobyClient.ContainerRemoveResult{}, errors.New("container does not exist")
}

// ContainerKill terminates the container process but does not remove the container from the docker host.
func (d *FakeDockerClient) ContainerKill(ctx context.Context, container string, options mobyClient.ContainerKillOptions) (mobyClient.ContainerKillResult, error) {
	return mobyClient.ContainerKillResult{}, nil
}

// ContainerStart sends a request to the docker daemon to start a container.
func (d *FakeDockerClient) ContainerStart(ctx context.Context, container string, options mobyClient.ContainerStartOptions) (mobyClient.ContainerStartResult, error) {
	d.Calls = append(d.Calls, "start")
	return mobyClient.ContainerStartResult{}, nil
}

// ImagePull requests the docker host to pull an image from a remote registry.
func (d *FakeDockerClient) ImagePull(ctx context.Context, ref string, options mobyClient.ImagePullOptions) (mobyClient.ImagePullResponse, error) {
	d.Calls = append(d.Calls, "pull")

	if d.PullFail != nil {
		return nil, d.PullFail
	}

	return imagePullResponseImpl{}, nil
}

type imagePullResponseImpl struct {
}

func (i imagePullResponseImpl) Read(p []byte) (n int, err error) {
	return 0, io.EOF
}

func (i imagePullResponseImpl) Close() error {
	return nil
}

func (i imagePullResponseImpl) JSONMessages(ctx context.Context) iter.Seq2[jsonstream.Message, error] {
	return func(yield func(jsonstream.Message, error) bool) {
		yield(jsonstream.Message{}, errors.New("function JSONMessages() not implemented"))
	}
}

func (i imagePullResponseImpl) Wait(ctx context.Context) error {
	return errors.New("function Wait() not implemented")
}

// ImageRemove removes an image from the docker host.
func (d *FakeDockerClient) ImageRemove(ctx context.Context, imageID string, options mobyClient.ImageRemoveOptions) (mobyClient.ImageRemoveResult, error) {
	d.Calls = append(d.Calls, "remove_image")

	if _, exists := d.Images[imageID]; exists {
		delete(d.Images, imageID)
		return mobyClient.ImageRemoveResult{}, nil
	}
	return mobyClient.ImageRemoveResult{}, errors.New("image does not exist")
}

// ServerVersion returns information of the docker client and server host.
func (d *FakeDockerClient) ServerVersion(ctx context.Context, options mobyClient.ServerVersionOptions) (mobyClient.ServerVersionResult, error) {
	return mobyClient.ServerVersionResult{}, nil
}

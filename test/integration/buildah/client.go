package buildah

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	dockertypes "github.com/docker/docker/api/types"
	dockercontainer "github.com/docker/docker/api/types/container"
	dockernetwork "github.com/docker/docker/api/types/network"
	"github.com/openshift/source-to-image/pkg/buildah"
	s2itar "github.com/openshift/source-to-image/pkg/tar"
	"github.com/openshift/source-to-image/pkg/test/fs"
)

// Client meant to replace docker.Client during integration tests. Is implementing the methods that
// are strictly necessary for test/integration/docker against buildah container manager.
type Client struct {
	manager *buildah.Buildah
}

// ContainerAttach not implemented.
func (c *Client) ContainerAttach(
	ctx context.Context,
	container string,
	options dockertypes.ContainerAttachOptions,
) (dockertypes.HijackedResponse, error) {
	panic("ContainerAttach")
}

// ContainerCommit not implemented.
func (c *Client) ContainerCommit(
	ctx context.Context,
	container string,
	options dockertypes.ContainerCommitOptions,
) (dockertypes.IDResponse, error) {
	panic("ContainerCommit")
}

// ContainerCreate execute "buildah from" to create a new container.
func (c *Client) ContainerCreate(
	ctx context.Context,
	config *dockercontainer.Config,
	hostConfig *dockercontainer.HostConfig,
	networkingConfig *dockernetwork.NetworkingConfig,
	containerName string,
) (dockercontainer.ContainerCreateCreatedBody, error) {
	var err error
	container := dockercontainer.ContainerCreateCreatedBody{}
	container.ID, err = c.manager.From(config.Image)
	return container, err
}

// ContainerInspect not implemented.
func (c *Client) ContainerInspect(
	ctx context.Context,
	container string,
) (dockertypes.ContainerJSON, error) {
	panic("ContainerInspect")
}

// ContainerRemove alias to manager's RemoveContainer.
func (c *Client) ContainerRemove(
	ctx context.Context,
	container string,
	options dockertypes.ContainerRemoveOptions,
) error {
	return c.manager.RemoveContainer(container)
}

// ContainerStart noop.
func (c *Client) ContainerStart(
	ctx context.Context,
	container string,
	options dockertypes.ContainerStartOptions,
) error {
	return nil
}

// ContainerKill noop.
func (c *Client) ContainerKill(ctx context.Context, container, signal string) error {
	return nil
}

// ContainerWait noop.
func (c *Client) ContainerWait(
	ctx context.Context,
	container string,
	condition dockercontainer.WaitCondition,
) (<-chan dockercontainer.ContainerWaitOKBody, <-chan error) {
	waitC := make(chan dockercontainer.ContainerWaitOKBody, 1)
	errC := make(chan error, 1)
	waitC <- dockercontainer.ContainerWaitOKBody{StatusCode: 0}
	return waitC, errC
}

// CopyToContainer not implemented.
func (c *Client) CopyToContainer(
	ctx context.Context,
	container, path string,
	content io.Reader,
	opts dockertypes.CopyToContainerOptions,
) error {
	panic("CopyToContainer")
}

// CopyFromContainer copy a file from container using manager's DownloadFromContainer, it returns a
// tar stream that's later extracted on a temporary directory.
func (c *Client) CopyFromContainer(
	ctx context.Context,
	container,
	srcPath string,
) (io.ReadCloser, dockertypes.ContainerPathStat, error) {
	// empty stat, meaning file or directory is not found
	stat := dockertypes.ContainerPathStat{}

	if container == "" {
		return nil, stat, fmt.Errorf("empty container name, can't continue")
	}

	var err error

	// downloading data from container on buffer, as tarball bytes
	tarBuffer := bytes.NewBuffer([]byte(""))
	if err = c.manager.DownloadFromContainer(srcPath, tarBuffer, container); err != nil {
		return nil, stat, err
	}
	// when buffer is empty, returning empty stat
	if len(tarBuffer.Bytes()) == 0 {
		return nil, stat, nil
	}

	// creating a temporary directory to receive the contents copied over from container
	tempDir, err := ioutil.TempDir("", fmt.Sprintf("s2i-copy-from-%s-", container))
	if err != nil {
		return nil, stat, err
	}
	defer os.RemoveAll(tempDir)

	// content download from container is a tar stream, therefore saving on buffer and directly
	// extracting on temporary folder
	fs := &fs.FakeFileSystem{}
	tar := s2itar.New(fs)
	if err = tar.ExtractTarStream(tempDir, tarBuffer); err != nil {
		return nil, stat, err
	}

	// checking for the existence of target file in temporary directory
	filename := filepath.Base(srcPath)
	pathInTempDir := path.Join(tempDir, filename)
	tempStat, err := os.Stat(pathInTempDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, stat, nil
		}
		return nil, stat, err
	}

	stat.Name = filename
	stat.Mtime = tempStat.ModTime()
	stat.Size = tempStat.Size()

	if tempStat.IsDir() {
		return nil, stat, nil
	}

	f, err := os.Open(pathInTempDir)
	if err != nil {
		return nil, stat, err
	}
	defer f.Close()

	return ioutil.NopCloser(f), stat, nil
}

// ImageBuild not implemented.
func (c *Client) ImageBuild(
	ctx context.Context,
	buildContext io.Reader,
	options dockertypes.ImageBuildOptions,
) (dockertypes.ImageBuildResponse, error) {
	panic("ImageBuild")
}

// ImageInspectWithRaw not implemented.
func (c *Client) ImageInspectWithRaw(
	ctx context.Context,
	image string,
) (dockertypes.ImageInspect, []byte, error) {
	imageInspect := dockertypes.ImageInspect{
		Config: &dockercontainer.Config{
			OnBuild: []string{},
		},
	}

	imageMetadata, err := buildah.InspectImage(image)
	if err != nil {
		return imageInspect, nil, err
	}

	imageInspect.ID = image
	if imageMetadata.Docker.Config.OnBuild != nil {
		imageInspect.Config.OnBuild = imageMetadata.Docker.Config.OnBuild
	}
	return imageInspect, nil, nil
}

// ImagePull not implemented.
func (c *Client) ImagePull(
	ctx context.Context,
	ref string,
	options dockertypes.ImagePullOptions,
) (io.ReadCloser, error) {
	panic("ImagePull")
}

// ImageRemove not implemented.
func (c *Client) ImageRemove(
	ctx context.Context,
	image string,
	options dockertypes.ImageRemoveOptions,
) ([]dockertypes.ImageDeleteResponseItem, error) {
	panic("ImageRemove")
}

// ServerVersion not implemented.
func (c *Client) ServerVersion(ctx context.Context) (dockertypes.Version, error) {
	panic("ServerVersion")
}

// NewClient instantiate a new buildah based client, using a container manager instance.
func NewClient(manager *buildah.Buildah) *Client {
	return &Client{manager: manager}
}

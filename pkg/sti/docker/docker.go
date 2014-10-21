package docker

import (
	"io"
	"log"
	"strings"

	"github.com/fsouza/go-dockerclient"
	"github.com/openshift/source-to-image/pkg/sti/errors"
)

// Docker is the interface between STI and the Docker client
// It contains higher level operations called from the STI
// build or usage commands
type Docker interface {
	IsImageInLocalRegistry(imageName string) (bool, error)
	RemoveContainer(id string) error
	GetDefaultUrl(image string) (string, error)
	RunContainer(opts RunContainerOptions) error
	GetImageId(image string) (string, error)
	CommitContainer(opts CommitContainerOptions) (string, error)
	RemoveImage(name string) error
}

// DockerClient contains all methods called on the go Docker
// client.
type DockerClient interface {
	RemoveImage(name string) error
	InspectImage(name string) (*docker.Image, error)
	PullImage(opts docker.PullImageOptions, auth docker.AuthConfiguration) error
	CreateContainer(opts docker.CreateContainerOptions) (*docker.Container, error)
	AttachToContainer(opts docker.AttachToContainerOptions) error
	StartContainer(id string, hostConfig *docker.HostConfig) error
	WaitContainer(id string) (int, error)
	RemoveContainer(opts docker.RemoveContainerOptions) error
	CommitContainer(opts docker.CommitContainerOptions) (*docker.Image, error)
	CopyFromContainer(opts docker.CopyFromContainerOptions) error
}

type stiDocker struct {
	client  DockerClient
	verbose bool
}

type postExecuteFunc func(containerId string, cmd []string) error

// RunContainerOptions are options passed in to the RunContainer method
type RunContainerOptions struct {
	Image     string
	PullImage bool
	Command   string
	Env       []string
	Stdin     io.Reader
	Stdout    io.Writer
	Stderr    io.Writer
	Started   chan struct{}
	PostExec  postExecuteFunc
}

// CommitContainerOptions are options passed in to the CommitContainer method
type CommitContainerOptions struct {
	ContainerID string
	Repository  string
	Command     []string
	Env         []string
}

// NewDocker creates a new implementation of the STI Docker interface
func NewDocker(endpoint string, verbose bool) (Docker, error) {
	client, err := docker.NewClient(endpoint)
	if err != nil {
		return nil, err
	}
	return &stiDocker{
		client:  client,
		verbose: verbose,
	}, nil
}

// IsImageInLocalRegistry determines whether the supplied image is in the local registry.
func (h *stiDocker) IsImageInLocalRegistry(imageName string) (bool, error) {
	image, err := h.client.InspectImage(imageName)

	if image != nil {
		return true, nil
	} else if err == docker.ErrNoSuchImage {
		return false, nil
	}

	return false, err
}

// CheckAndPull pulls an image into the local registry
func (h *stiDocker) CheckAndPull(imageName string) (*docker.Image, error) {
	image, err := h.client.InspectImage(imageName)
	if err != nil && err != docker.ErrNoSuchImage {
		return nil, errors.ErrPullImageFailed
	}

	if image == nil {
		log.Printf("Pulling image %s\n", imageName)

		err = h.client.PullImage(docker.PullImageOptions{Repository: imageName}, docker.AuthConfiguration{})
		if err != nil {
			return nil, errors.ErrPullImageFailed
		}

		image, err = h.client.InspectImage(imageName)
		if err != nil {
			return nil, err
		}
	} else if h.verbose {
		log.Printf("Image %s available locally\n", imageName)
	}

	return image, nil
}

// RemoveContainer removes a container and its associated volumes.
func (h *stiDocker) RemoveContainer(id string) error {
	return h.client.RemoveContainer(docker.RemoveContainerOptions{id, true, true})
}

// GetDefaultUrl finds a script URL in the given image's metadata
func (h *stiDocker) GetDefaultUrl(image string) (string, error) {
	imageMetadata, err := h.CheckAndPull(image)
	if err != nil {
		return "", err
	}
	var defaultScriptsUrl string
	env := append(imageMetadata.ContainerConfig.Env, imageMetadata.Config.Env...)
	for _, v := range env {
		if strings.HasPrefix(v, "STI_SCRIPTS_URL=") {
			defaultScriptsUrl = v[len("STI_SCRIPTS_URL="):]
			break
		}
	}
	if h.verbose {
		log.Printf("Image contains default script url '%s'", defaultScriptsUrl)
	}
	return defaultScriptsUrl, nil
}

// RunContainer executes an image specified in the options with the ability
// to stream input or output
func (h *stiDocker) RunContainer(opts RunContainerOptions) error {
	// get info about the specified image
	var imageMetadata *docker.Image
	var err error
	if opts.PullImage {
		imageMetadata, err = h.CheckAndPull(opts.Image)
	} else {
		imageMetadata, err = h.client.InspectImage(opts.Image)
	}
	if err != nil {
		log.Printf("Error: Unable to get image metadata for %s: %v", opts.Image, err)
		return err
	}

	cmd := imageMetadata.Config.Cmd
	cmd = append(cmd, opts.Command)
	config := docker.Config{
		Image: opts.Image,
		Cmd:   cmd,
	}

	if opts.Env != nil {
		config.Env = opts.Env
	}

	if opts.Stdin != nil {
		config.OpenStdin = true
		config.StdinOnce = true
	}

	if opts.Stdout != nil {
		config.AttachStdout = true
	}

	if h.verbose {
		log.Printf("Creating container using config: %+v\n", config)
	}

	container, err := h.client.CreateContainer(docker.CreateContainerOptions{Name: "", Config: &config})
	if err != nil {
		return err
	}
	defer h.RemoveContainer(container.ID)

	if h.verbose {
		log.Printf("Attaching to container")
	}
	attached := make(chan struct{})
	attachOpts := docker.AttachToContainerOptions{
		Container: container.ID,
		Success:   attached,
		Stream:    true,
	}
	if opts.Stdin != nil {
		attachOpts.InputStream = opts.Stdin
		attachOpts.Stdin = true
	} else if opts.Stdout != nil {
		attachOpts.OutputStream = opts.Stdout
		attachOpts.Stdout = true
	}

	go h.client.AttachToContainer(attachOpts)
	attached <- <-attached

	// If attaching both stdin and stdout, attach stdout in
	// a second thread
	if opts.Stdin != nil && opts.Stdout != nil {
		attached2 := make(chan struct{})
		attachOpts2 := docker.AttachToContainerOptions{
			Container:    container.ID,
			Success:      attached2,
			Stream:       true,
			OutputStream: opts.Stdout,
			Stdout:       true,
		}
		if opts.Stderr != nil {
			attachOpts2.Stderr = true
			attachOpts2.ErrorStream = opts.Stderr
		}
		go h.client.AttachToContainer(attachOpts2)
		attached2 <- <-attached2
	}

	if h.verbose {
		log.Printf("Starting container")
	}
	err = h.client.StartContainer(container.ID, nil)
	if err != nil {
		return err
	}
	if opts.Started != nil {
		opts.Started <- struct{}{}
	}

	if h.verbose {
		log.Printf("Waiting for container")
	}
	exitCode, err := h.client.WaitContainer(container.ID)
	if err != nil {
		return err
	}
	if h.verbose {
		log.Printf("Container exited")
	}

	if exitCode != 0 {
		return errors.StiContainerError{exitCode}
	}

	if opts.PostExec != nil {
		if h.verbose {
			log.Printf("Invoking postExecution function")
		}
		err = opts.PostExec(container.ID, imageMetadata.Config.Cmd)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetImageId retrives the ID of the image identified by name
func (h *stiDocker) GetImageId(imageName string) (string, error) {
	image, err := h.client.InspectImage(imageName)
	if err == nil {
		return image.ID, nil
	} else {
		return "", err
	}
}

func (h *stiDocker) CommitContainer(opts CommitContainerOptions) (string, error) {
	dockerOpts := docker.CommitContainerOptions{
		Container:  opts.ContainerID,
		Repository: opts.Repository,
	}
	if opts.Command != nil {
		config := docker.Config{
			Cmd: opts.Command,
			Env: opts.Env,
		}
		dockerOpts.Run = &config
		if h.verbose {
			log.Printf("Commiting container with config: %+v\n", config)
		}
	}

	image, err := h.client.CommitContainer(dockerOpts)

	if err != nil && image != nil {
		return image.ID, nil
	} else {
		return "", err
	}
}

func (h *stiDocker) RemoveImage(imageID string) error {
	return h.client.RemoveImage(imageID)
}

package sti

import (
	"log"
	"os"

	"github.com/fsouza/go-dockerclient"
)

// Request contains essential fields for any request: a Configuration, a base image, and an
// optional runtime image.
type Request struct {
	BaseImage    string
	RuntimeImage string

	DockerSocket  string
	DockerTimeout int
	WorkingDir    string
	Debug         bool
}

// requestHandler encapsulates dependencies needed to fulfill requests.
type requestHandler struct {
	dockerClient *docker.Client
	debug        bool
}

type STIResult struct {
	Success  bool
	Messages []string
}

// Returns a new handler for a given request.
func newHandler(req Request) (*requestHandler, error) {
	if req.Debug {
		log.Printf("Using docker socket: %s\n", req.DockerSocket)
	}

	dockerClient, err := docker.NewClient(req.DockerSocket)
	if err != nil {
		return nil, ErrDockerConnectionFailed
	}

	return &requestHandler{dockerClient, req.Debug}, nil
}

// Determines whether the supplied image is in the local registry.
func (h requestHandler) isImageInLocalRegistry(imageName string) (bool, error) {
	image, err := h.dockerClient.InspectImage(imageName)

	if image != nil {
		return true, nil
	} else if err == docker.ErrNoSuchImage {
		return false, nil
	}

	return false, err
}

// Pull an image into the local registry
func (h requestHandler) checkAndPull(imageName string) (*docker.Image, error) {
	image, err := h.dockerClient.InspectImage(imageName)
	if err != nil {
		return nil, ErrPullImageFailed
	}

	if image == nil {
		if h.debug {
			log.Printf("Pulling image %s\n", imageName)
		}

		err = h.dockerClient.PullImage(docker.PullImageOptions{Repository: imageName}, docker.AuthConfiguration{})
		if err != nil {
			return nil, ErrPullImageFailed
		}

		image, err = h.dockerClient.InspectImage(imageName)
		if err != nil {
			return nil, err
		}
	} else if h.debug {
		log.Printf("Image %s available locally\n", imageName)
	}

	return image, nil
}

// Creates a container from a given image name and returns the ID of the created container.
func (h requestHandler) containerFromImage(imageName string) (*docker.Container, error) {
	config := docker.Config{Image: imageName, AttachStdout: false, AttachStderr: false, Cmd: []string{"/bin/true"}}
	container, err := h.dockerClient.CreateContainer(docker.CreateContainerOptions{Name: "", Config: &config})
	if err != nil {
		return nil, err
	}

	err = h.dockerClient.StartContainer(container.ID, &docker.HostConfig{})
	if err != nil {
		return nil, err
	}

	exitCode, err := h.dockerClient.WaitContainer(container.ID)
	if err != nil {
		return nil, err
	}

	if exitCode != 0 {
		log.Printf("Container exit code: %d\n", exitCode)
		return nil, ErrCreateContainerFailed
	}

	return container, nil
}

// Remove a container and its associated volumes.
func (h requestHandler) removeContainer(id string) {
	h.dockerClient.RemoveContainer(docker.RemoveContainerOptions{id, true, true})
}

// Commit the container with the given ID with the given tag.
func (h requestHandler) commitContainer(id, tag string) error {
	// TODO: commit message / author?
	_, err := h.dockerClient.CommitContainer(docker.CommitContainerOptions{Container: id, Repository: tag})
	return err
}

// If image is not present locally, pull it from Docker registry
func (h requestHandler) pullImage(image string) (*docker.Image, error) {
	log.Printf("Image %s was not found locally, pulling it from Docker registry", image)
	err := h.dockerClient.PullImage(docker.PullImageOptions{Repository: image, OutputStream: os.Stdout}, docker.AuthConfiguration{})
	if err != nil {
		return nil, err
	}
	return h.dockerClient.InspectImage(image)
}

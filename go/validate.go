package sti

import (
	"fmt"
	"log"

	"github.com/fsouza/go-dockerclient"
)

// Describes a request to validate an images for use in an sti build.
type ValidateRequest struct {
	Request
	Incremental bool
}

type ValidateResult STIResult

// Records the result of a validation on a ValidationResult.
func (res *ValidateResult) recordValidation(what string, image string, valid bool) {
	if !valid {
		res.Success = false
		res.Messages = append(res.Messages, fmt.Sprintf("%s %s failed validation", what, image))
	} else {
		res.Messages = append(res.Messages, fmt.Sprintf("%s %s passes validation", what, image))
	}
}

// Service the supplied ValidateRequest and return a ValidateResult.
func Validate(req ValidateRequest) (*ValidateResult, error) {
	c, err := newHandler(req.Request)
	if err != nil {
		return nil, err
	}

	result := &ValidateResult{Success: true}

	if req.RuntimeImage != "" {
		valid, err := c.validateImage(req.BaseImage, false)
		if err != nil {
			return nil, err
		}
		result.recordValidation("Base image", req.BaseImage, valid)

		valid, err = c.validateImage(req.RuntimeImage, true)
		if err != nil {
			return nil, err
		}
		result.recordValidation("Runtime image", req.RuntimeImage, valid)
	} else {
		valid, err := c.validateImage(req.BaseImage, req.Incremental)
		if err != nil {
			return nil, err
		}
		result.recordValidation("Base image", req.BaseImage, valid)
	}

	return result, nil
}

func (h requestHandler) validateImage(imageName string, incremental bool) (bool, error) {
	log.Printf("Validating image %s, incremental: %t\n", imageName, incremental)
	image, err := h.checkAndPull(imageName)
	if err != nil {
		return false, err
	}

	if h.debug {
		log.Printf("Pulled image %s: {%+v}", imageName, image)
	}

	if imageHasEntryPoint(image) {
		log.Printf("ERROR: Image %s has a configured entrypoint and is incompatible with sti\n", imageName)
		return false, nil
	}

	files := []string{"/usr/bin/prepare", "/usr/bin/run"}

	if incremental {
		files = append(files, "/usr/bin/save-artifacts")
	}

	valid, err := h.validateRequiredFiles(imageName, files)
	if err != nil {
		return false, err
	}

	return valid, nil
}

func (h requestHandler) validateRequiredFiles(imageName string, files []string) (bool, error) {
	container, err := h.containerFromImage(imageName)
	if err != nil {
		return false, ErrCreateContainerFailed
	}
	defer h.dockerClient.RemoveContainer(docker.RemoveContainerOptions{container.ID, true, true})

	for _, file := range files {
		if !FileExistsInContainer(h.dockerClient, container.ID, file) {
			log.Printf("ERROR: Image %s is missing %s\n", imageName, file)
			return false, nil
		} else if h.debug {
			log.Printf("OK: Image %s contains file %s\n", imageName, file)
		}
	}

	return true, nil
}

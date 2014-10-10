package sti

import (
	"io"
	"log"
	"path/filepath"

	"github.com/openshift/source-to-image/pkg/sti/docker"
	"github.com/openshift/source-to-image/pkg/sti/errors"
	"github.com/openshift/source-to-image/pkg/sti/git"
	"github.com/openshift/source-to-image/pkg/sti/util"
)

type Builder struct {
	handler buildHandlerInterface
}

type buildHandlerInterface interface {
	cleanup()
	setup(required []string, optional []string) error
	determineIncremental() error
	buildRequest() *STIRequest
	buildResult() *STIResult
	saveArtifacts() error
	fetchSource() error
	execute(command string) error
}

type buildHandler struct {
	*requestHandler
	git             git.Git
	callbackInvoker util.CallbackInvoker
}

func NewBuilder(req *STIRequest) (*Builder, error) {
	handler, err := newBuildHandler(req)
	if err != nil {
		return nil, err
	}
	return &Builder{
		handler: handler,
	}, nil
}

func newBuildHandler(req *STIRequest) (*buildHandler, error) {
	rh, err := newRequestHandler(req)
	if err != nil {
		return nil, err
	}
	bh := &buildHandler{
		requestHandler:  rh,
		git:             git.NewGit(req.Verbose),
		callbackInvoker: util.NewCallbackInvoker(),
	}
	rh.postExecutor = bh
	return bh, nil
}

// Build processes a Request and returns a *Result and an error.
// An error represents a failure performing the build rather than a failure
// of the build itself.  Callers should check the Success field of the result
// to determine whether a build succeeded or not.
func (b *Builder) Build() (*STIResult, error) {
	bh := b.handler
	defer bh.cleanup()

	err := bh.setup([]string{"assemble", "run"}, []string{"save-artifacts"})
	if err != nil {
		return nil, err
	}

	err = bh.determineIncremental()
	if err != nil {
		return nil, err
	}
	if bh.buildRequest().incremental {
		log.Printf("Existing image for tag %s detected for incremental build.\n", bh.buildRequest().Tag)
	} else {
		log.Println("Clean build will be performed")
	}

	if bh.buildRequest().Verbose {
		log.Printf("Performing source build from %s\n", bh.buildRequest().Source)
	}

	if bh.buildRequest().incremental {
		err = bh.saveArtifacts()
		if err != nil {
			return nil, err
		}
	}

	err = bh.fetchSource()
	if err != nil {
		return nil, err
	}

	err = bh.execute("assemble")
	if err != nil {
		return nil, err
	}

	return bh.buildResult(), nil
}

func (h *buildHandler) postExecute(containerID string, cmd []string) error {
	previousImageId := ""
	var err error
	if h.request.incremental && h.request.RemovePreviousImage {
		previousImageId, err = h.getPreviousImageId()
		if err != nil {
			log.Printf("Error retrieving previous image's metadata: %s", err.Error())
		}
	}

	opts := docker.CommitContainerOptions{
		Command:     append(cmd, "run"),
		Env:         h.generateConfigEnv(),
		ContainerID: containerID,
		Repository:  h.request.Tag,
	}
	imageID, err := h.docker.CommitContainer(opts)
	if err != nil {
		return errors.ErrBuildFailed
	}

	h.result.ImageID = imageID
	log.Printf("Tagged %s as %s\n", imageID, h.request.Tag)

	if h.request.incremental && h.request.RemovePreviousImage && previousImageId != "" {
		log.Printf("Removing previously-tagged image %s\n", previousImageId)
		err = h.docker.RemoveImage(previousImageId)
		if err != nil {
			log.Printf("Unable to remove previous image: %s\n", err.Error())
		}
	}

	if h.request.CallbackUrl != "" {
		h.result.Messages = h.callbackInvoker.ExecuteCallback(h.request.CallbackUrl,
			h.result.Success, h.result.Messages)
	}

	return nil
}

func (h *buildHandler) getPreviousImageId() (string, error) {
	return h.docker.GetImageId(h.request.Tag)
}

func (h *buildHandler) determineIncremental() error {
	var err error
	incremental := !h.request.Clean

	if incremental {
		// can only do incremental build if runtime image exists
		incremental, err = h.docker.IsImageInLocalRegistry(h.request.Tag)
		if err != nil {
			return err
		}
	}
	if incremental {
		// check if a save-artifacts script exists in anything provided to the build
		// without it, we cannot do incremental builds
		incremental = h.fs.Exists(
			filepath.Join(h.request.workingDir, "upload", "scripts", "save-artifacts"))
	}

	h.request.incremental = incremental

	return nil
}

func (h *buildHandler) saveArtifacts() error {
	artifactTmpDir := filepath.Join(h.request.workingDir, "upload", "artifacts")
	err := h.fs.Mkdir(artifactTmpDir)
	if err != nil {
		return err
	}

	image := h.request.Tag

	log.Printf("Saving build artifacts from image %s to path %s\n", image, artifactTmpDir)

	reader, writer := io.Pipe()
	started := make(chan struct{})
	cancelExtract := make(chan error)
	extractError := make(chan error)

	opts := docker.RunContainerOptions{
		Image:   image,
		Command: "save-artifacts",
		Stdout:  writer,
		Started: started,
	}

	// Launch thread that will wait for container start
	// and start extracting output tar from stream
	go func() {
		select {
		case <-started:
			// Container has started, start extracting
			defer reader.Close()
			err := h.tar.ExtractTarStream(artifactTmpDir, reader)
			if err != nil {
				log.Printf("An error occurred while extracting the tar stream.")
			}
			extractError <- err
		case <-cancelExtract:
			// An error occurred before we could start the
			// container. Simply exit this thread.
			return
		}
	}()

	err = h.docker.RunContainer(opts)
	writer.Close()
	if err != nil {
		// Get result of extract or cancel it if still waiting for
		// container start
		select {
		case <-extractError: // Extract finished and we have an error result. In case
			// both RunContainer and extract return errors, the error
			// from RunContainer will be returned. The extract error
			// will be ignored
		case cancelExtract <- err: // Extract hasn't started and needs to be canceled
		}

		switch e := err.(type) {
		case errors.StiContainerError:
			if h.request.Verbose {
				log.Printf("Exit code: %d", e.ExitCode)
			}
			return errors.ErrSaveArtifactsFailed
		default:
			return err
		}
	} else {
		// In the case of no error from RunContainer
		// wait for the result of extract.
		select {
		case err = <-extractError:
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (h *buildHandler) fetchSource() error {
	targetSourceDir := filepath.Join(h.request.workingDir, "upload", "src")

	log.Printf("Downloading %s to directory %s\n", h.request.Source, targetSourceDir)

	if h.git.ValidCloneSpec(h.request.Source) {
		err := h.git.Clone(h.request.Source, targetSourceDir)
		if err != nil {
			log.Printf("Git clone failed: %+v", err)
			return err
		}

		if h.request.Ref != "" {
			log.Printf("Checking out ref %s", h.request.Ref)

			err := h.git.Checkout(targetSourceDir, h.request.Ref)
			if err != nil {
				return err
			}
		}
	} else {
		h.fs.Copy(h.request.Source, targetSourceDir)
	}

	return nil
}

func (h *buildHandler) buildRequest() *STIRequest {
	return h.request
}

func (h *buildHandler) buildResult() *STIResult {
	return h.result
}

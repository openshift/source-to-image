package sti

import (
	"io"
	"path/filepath"
	"regexp"

	"github.com/golang/glog"
	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/docker"
	"github.com/openshift/source-to-image/pkg/errors"
	"github.com/openshift/source-to-image/pkg/util"
)

type buildHandlerInterface interface {
	cleanup()
	setup(required []api.Script, optional []api.Script) error
	determineIncremental() error
	Request() *api.Request
	Result() *api.Result
	saveArtifacts() error
	fetchSource() error
	execute(command api.Script) error
	wasExpectedError(text string) bool
	build() error
}

type buildHandler struct {
	*requestHandler
	callbackInvoker util.CallbackInvoker
}

type postExecutor interface {
	PostExecute(containerID string, location string) error
}

func NewBuildHandler(req *api.Request) (*buildHandler, error) {
	rh, err := NewRequestHandler(req)
	if err != nil {
		return nil, err
	}
	bh := &buildHandler{
		requestHandler:  rh,
		callbackInvoker: util.NewCallbackInvoker(),
	}
	rh.postExecutor = bh
	return bh, nil
}

// wasExpectedError is used for determining whether the error that appeared
// authorizes us to do the additional build injecting the scripts and sources.
func (h *buildHandler) wasExpectedError(text string) bool {
	tar, _ := regexp.MatchString(`.*tar.*not found`, text)
	sh, _ := regexp.MatchString(`.*/bin/sh.*no such file or directory`, text)
	return tar || sh
}

// PostExecute executes post-build operations
func (h *buildHandler) PostExecute(containerID string, location string) error {
	var (
		err             error
		previousImageID string
	)
	if h.request.Incremental && h.request.RemovePreviousImage {
		if previousImageID, err = h.docker.GetImageID(h.request.Tag); err != nil {
			glog.Errorf("Error retrieving previous image's metadata: %v", err)
		}
	}

	cmd := []string{}
	opts := docker.CommitContainerOptions{
		Command:     append(cmd, filepath.Join(location, string(api.Run))),
		Env:         h.generateConfigEnv(),
		ContainerID: containerID,
		Repository:  h.request.Tag,
	}
	imageID, err := h.docker.CommitContainer(opts)
	if err != nil {
		return errors.NewBuildError(h.request.Tag, err)
	}

	h.result.ImageID = imageID
	glog.V(1).Infof("Tagged %s as %s", imageID, h.request.Tag)

	if h.request.Incremental && h.request.RemovePreviousImage && previousImageID != "" {
		glog.V(1).Infof("Removing previously-tagged image %s", previousImageID)
		if err = h.docker.RemoveImage(previousImageID); err != nil {
			glog.Errorf("Unable to remove previous image: %v", err)
		}
	}

	if h.request.CallbackURL != "" {
		h.result.Messages = h.callbackInvoker.ExecuteCallback(h.request.CallbackURL,
			h.result.Success, h.result.Messages)
	}

	glog.Infof("Successfully built %s", h.request.Tag)
	return nil
}

func (h *buildHandler) determineIncremental() (err error) {
	h.request.Incremental = false
	if h.request.Clean {
		return
	}

	// can only do incremental build if runtime image exists
	previousImageExists, err := h.docker.IsImageInLocalRegistry(h.request.Tag)
	if err != nil {
		return
	}

	// we're assuming save-artifacts to exists for embedded scripts (if not we'll
	// warn a user upon container failure and proceed with clean build)
	// for external save-artifacts - check its existence
	saveArtifactsExists := !h.request.ExternalOptionalScripts ||
		h.fs.Exists(filepath.Join(h.request.WorkingDir, "upload", "scripts", string(api.SaveArtifacts)))
	h.request.Incremental = previousImageExists && saveArtifactsExists
	return nil
}

func (h *buildHandler) saveArtifacts() (err error) {
	artifactTmpDir := filepath.Join(h.request.WorkingDir, "upload", "artifacts")
	if err = h.fs.Mkdir(artifactTmpDir); err != nil {
		return err
	}

	image := h.request.Tag
	reader, writer := io.Pipe()
	glog.V(1).Infof("Saving build artifacts from image %s to path %s", image, artifactTmpDir)
	extractFunc := func() error {
		defer reader.Close()
		return h.tar.ExtractTarStream(artifactTmpDir, reader)
	}

	opts := docker.RunContainerOptions{
		Image:           image,
		ExternalScripts: h.request.ExternalRequiredScripts,
		ScriptsURL:      h.request.ScriptsURL,
		Location:        h.request.Location,
		Command:         api.SaveArtifacts,
		Stdout:          writer,
		OnStart:         extractFunc,
	}
	err = h.docker.RunContainer(opts)
	writer.Close()
	if e, ok := err.(errors.ContainerError); ok {
		return errors.NewSaveArtifactsError(image, e.Output, err)
	}
	return err
}

func (h *buildHandler) Request() *api.Request {
	return h.request
}

func (h *buildHandler) Result() *api.Result {
	return h.result
}

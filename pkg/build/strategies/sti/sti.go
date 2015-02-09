package sti

import (
	"github.com/golang/glog"
	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/errors"
)

const (
	// maxErrorOutput is the maximum length of the error output saved for processing
	maxErrorOutput = 1024
	// defaultLocation is the default location of the scripts and sources in image
	defaultLocation = "/tmp"
)

var (
	// List of directories that needs to be present inside workign dir
	workingDirs = []string{
		"upload/scripts",
		"upload/src",
		"downloads/scripts",
		"downloads/defaultScripts",
	}
)

type STI struct {
	handler buildHandlerInterface
}

// NewSTI returns a new STI builder
func NewSTI(req *api.Request) (*STI, error) {
	handler, err := NewBuildHandler(req)
	if err != nil {
		return nil, err
	}
	return &STI{
		handler: handler,
	}, nil
}

// Build processes a Request and returns a *api.Result and an error.
// An error represents a failure performing the build rather than a failure
// of the build itself.  Callers should check the Success field of the result
// to determine whether a build succeeded or not.
func (b *STI) Build(request *api.Request) (*api.Result, error) {
	bh := b.handler
	defer bh.cleanup()

	glog.Infof("Building %s", bh.Request().Tag)
	err := bh.setup([]api.Script{api.Assemble, api.Run}, []api.Script{api.SaveArtifacts})
	if err != nil {
		return nil, err
	}

	err = bh.determineIncremental()
	if err != nil {
		return nil, err
	}
	if bh.Request().Incremental {
		glog.V(1).Infof("Existing image for tag %s detected for incremental build.", bh.Request().Tag)
	} else {
		glog.V(1).Infof("Clean build will be performed")
	}

	glog.V(2).Infof("Performing source build from %s", bh.Request().Source)
	if bh.Request().Incremental {
		if err = bh.saveArtifacts(); err != nil {
			glog.Warningf("Error saving previous build artifacts: %v", err)
			glog.Warning("Clean build will be performed!")
		}
	}

	glog.V(1).Infof("Building %s", bh.Request().Tag)
	if err := bh.execute(api.Assemble); err != nil {
		switch e := err.(type) {
		case errors.ContainerError:
			return b.handleContainerError(e)
		default:
			return nil, err
		}
	}

	return bh.Result(), nil
}

// handleContainerError is responsible for checking container output to see if
// the error is one of the expected that should trigger layered build
func (b *STI) handleContainerError(cerr errors.ContainerError) (*api.Result, error) {
	bh := b.handler
	if bh.wasExpectedError(cerr.Output) {
		glog.Warningf("Image %s does not have tar! Performing additional build to add the scripts and sources.",
			bh.Request().BaseImage)
		if err := bh.build(); err != nil {
			return nil, err
		}
		glog.V(2).Infof("Building %s using sti-enabled image", bh.Request().Tag)
		if err := bh.execute(api.Assemble); err != nil {
			glog.V(2).Infof("FOOOO\n")
			switch e := err.(type) {
			case errors.ContainerError:
				return nil, errors.NewAssembleError(bh.Request().Tag, e.Output, e)
			default:
				return nil, err
			}
		}
	} else {
		return nil, errors.NewAssembleError(bh.Request().Tag, cerr.Output, cerr)
	}

	return bh.Result(), nil
}

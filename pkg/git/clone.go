package git

import (
	"path/filepath"

	"github.com/golang/glog"
	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/util"
)

type Clone struct {
	Git
	util.FileSystem
}

// Download downloads the application source code from the GIT repository
// and checkout the Ref specified in the request.
func (c *Clone) Download(request *api.Request) error {
	targetSourceDir := filepath.Join(request.WorkingDir, "upload", "src")
	glog.V(1).Infof("Downloading %s to directory %s", request.Source, targetSourceDir)

	if c.ValidCloneSpec(request.Source) {
		if err := c.Clone(request.Source, targetSourceDir); err != nil {
			glog.Errorf("Git clone failed: %+v", err)
			return err
		}

		if request.Ref != "" {
			glog.V(1).Infof("Checking out ref %s", request.Ref)

			if err := c.Checkout(targetSourceDir, request.Ref); err != nil {
				return err
			}
		}
	} else if err := c.Copy(request.Source, targetSourceDir); err != nil {
		return err
	}

	return nil
}

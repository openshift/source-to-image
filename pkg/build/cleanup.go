package build

import (
	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/docker"
	"github.com/openshift/source-to-image/pkg/util/fs"
	utillog "github.com/openshift/source-to-image/pkg/util/log"
)

var log = utillog.StderrLog

// DefaultCleaner provides a cleaner for most STI build use-cases. It cleans the
// temporary directories created by STI build and it also cleans the temporary
// Docker images produced by LayeredBuild
type DefaultCleaner struct {
	fs     fs.FileSystem
	docker docker.Docker
}

// NewDefaultCleaner creates a new instance of the default Cleaner implementation
func NewDefaultCleaner(fs fs.FileSystem, docker docker.Docker) Cleaner {
	return &DefaultCleaner{
		fs:     fs,
		docker: docker,
	}
}

// Cleanup removes the temporary directories where the sources were stored for build.
func (c *DefaultCleaner) Cleanup(config *api.Config) {
	if config.PreserveWorkingDir {
		log.V(2).Infof("Temporary directory %q will be saved, not deleted", config.WorkingDir)
	} else {
		log.V(2).Infof("Removing temporary directory %s", config.WorkingDir)
		if err := c.fs.RemoveDirectory(config.WorkingDir); err != nil {
			log.Warningf("Error removing temporary directory %q: %v", config.WorkingDir, err)
		}
	}
	if config.LayeredBuild {
		// config.LayeredBuild is true only when layered build was finished successfully.
		// Also in this case config.BuilderImage contains name of the new just built image,
		// not the original one that was specified by the user.
		log.V(2).Infof("Removing temporary image %s", config.BuilderImage)
		if err := c.docker.RemoveImage(config.BuilderImage); err != nil {
			log.Warningf("Error removing temporary image %s: %v", config.BuilderImage, err)
		}
	}
}

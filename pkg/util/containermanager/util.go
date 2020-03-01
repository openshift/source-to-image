package containermanager

import (
	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/buildah"
	"github.com/openshift/source-to-image/pkg/docker"
)

// GetClient returns the container runtime client based on environment variable.
func GetClient(cfg *api.Config) (docker.Client, error) {
	var apiClient docker.Client
	var err error

	switch cfg.ContainerManager {
	case "buildah":
		apiClient = nil
	default:
		apiClient, err = docker.NewEngineAPIClient(cfg.DockerConfig)
		if err != nil {
			panic(err)
		}
	}
	return apiClient, err
}

// GetDocker returns the container runtime instance based on environment variable.
func GetDocker(client docker.Client, config *api.Config, authConfig api.AuthConfig) docker.Docker {
	switch config.ContainerManager {
	case "buildah":
		return buildah.NewBuildah(client)
	default:
		return docker.New(client, authConfig)
	}
}

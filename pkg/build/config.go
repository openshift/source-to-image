package build

import (
	"fmt"

	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/docker"
)

// GenerateConfigFromLabels generates the S2I Config struct from the Docker
// image labels.
func GenerateConfigFromLabels(config *api.Config) error {
	result, err := docker.GetBuilderImage(config)
	if err != nil {
		return err
	}
	labels := result.Image.ConfigLabels

	if builderVersion, ok := labels["io.openshift.builder-version"]; ok {
		config.BuilderImageVersion = builderVersion
		config.BuilderBaseImageVersion = labels["io.openshift.builder-base-version"]
	}

	config.ScriptsURL = labels[api.DefaultNamespace+"scripts-url"]
	if len(config.ScriptsURL) == 0 {
		// FIXME: Backward compatibility
		config.ScriptsURL = labels["io.s2i.scripts-url"]
	}

	config.Description = labels[api.KubernetesNamespace+"description"]
	config.DisplayName = labels[api.KubernetesNamespace+"display-name"]

	if builder, ok := labels[api.DefaultNamespace+"build.image"]; ok {
		config.BuilderImage = builder
	} else {
		return fmt.Errorf("Required label %q not found in image", api.DefaultNamespace+"build.image")
	}

	if repo, ok := labels[api.DefaultNamespace+"build.source-location"]; ok {
		config.Source = repo
	} else {
		return fmt.Errorf("Required label %q not found in image", api.DefaultNamespace+"source-location")
	}

	config.ContextDir = labels[api.DefaultNamespace+"build.source-context-dir"]
	config.Ref = labels[api.DefaultNamespace+"build.commit.ref"]

	return nil
}

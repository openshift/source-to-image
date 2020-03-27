package buildah

import (
	"encoding/json"
)

// Inspect parsed outcomes of "buildah inspect" calls.
type Inspect struct {
	FromImageID string        `json:"FromImageID"`
	Docker      InspectDocker `json:"Docker"`
}

// InspectDocker docker section of config instance.
type InspectDocker struct {
	Config InspectDockerConfig `json:"config"`
}

// InspectDockerConfig config section inside Docker config.
type InspectDockerConfig struct {
	User       string            `json:"User"`
	Env        []string          `json:"Env"`
	WorkingDir string            `json:"WorkingDir"`
	Entrypoint []string          `json:"Entrypoint"`
	OnBuild    []string          `json:"OnBuild"`
	Labels     map[string]string `json:"Labels"`
}

// InspectImage run "buildah inspect" and parse out returned json to compose a Inspect instance. It
// can return error in case of buildah does, and in case of not being able to parse out json output.
func InspectImage(image string) (*Inspect, error) {
	log.V(3).Infof("Inspecting image '%s' with buildah...", image)
	output, err := Execute([]string{buildahCmd, "inspect", image}, nil, false)
	if err != nil {
		return nil, err
	}

	imageMetadata := &Inspect{}
	err = json.Unmarshal(output, &imageMetadata)
	if err != nil {
		log.Errorf("Error parsing JSON output '%s': '%q'", output, err)
		return nil, err
	}
	return imageMetadata, nil
}

// +build integration

package buildah

import (
	"os"
	"testing"

	"github.com/openshift/source-to-image/pkg/api/constants"
	buildahpkg "github.com/openshift/source-to-image/pkg/buildah"
)

func TestInspect(t *testing.T) {
	if os.Getenv(constants.ContainerManagerEnv) != constants.BuildahContainerManager {
		t.Skip("skipping buildah-inspect integration tests")
		return
	}

	t.Run("InspectImage", buildahInspectImageTest)
}

func buildahInspectImageTest(t *testing.T) {
	imageMetadata, err := buildahpkg.InspectImage(exampleImage)
	if err != nil {
		t.Fail() // expect no errors
	}
	if imageMetadata == nil {
		t.Fail() // expect metdata
	} else {
		if imageMetadata.FromImageID == "" {
			t.Fail() // expect image ID
		}
		if imageMetadata.Docker.Config.User == "" {
			t.Fail() // expect to find user
		}
		if len(imageMetadata.Docker.Config.Env) == 0 {
			t.Fail() // expect to find environment variables
		}
	}
}

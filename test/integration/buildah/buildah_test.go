// +build integration

package buildah

import (
	"os"
	"path"
	"testing"

	"github.com/openshift/source-to-image/pkg/api/constants"
	buildahpkg "github.com/openshift/source-to-image/pkg/buildah"
	"github.com/openshift/source-to-image/pkg/docker"
	"github.com/openshift/source-to-image/pkg/util/fs"
)

const (
	busyboxImage = "docker.io/busybox:latest"
	exampleImage = "nginx-centos7:testing"
)

func TestBuildah(t *testing.T) {
	if os.Getenv(constants.ContainerManagerEnv) != "buildah" {
		t.Skip("skipping buildah integration tests")
		return
	}

	t.Run("buildImage", buildImageTest)
	t.Run("inspectImage", inspectImageTest)
	t.Run("IsImageInLocalRegistry", isImageInLocalRegistryTest)
	t.Run("IsImageOnBuild", isImageOnBuildTest)
	t.Run("GetOnBuild", getOnBuildTest)
	t.Run("GetScriptsURL", getScriptsURLTest)
	t.Run("GetAssembleInputFiles", getAssembleInputFilesTest)
	t.Run("GetAssembleRuntimeUser", getAssembleRuntimeUserTest)
	t.Run("GetImageName", getImageNameTest)
	t.Run("GetImageID", getImageIDTest)
	t.Run("GetImageWorkdir", getImageWorkdirTest)
	t.Run("CheckImage", checkImageTest)
	t.Run("PullImage", pullImageTest)
	t.Run("CheckAndPullImage", checkAndPullImageTest)
	t.Run("GetImageUser", getImageUserTest)
	t.Run("GetImageEntrypoint", getImageEntrypointTest)
	t.Run("GetLabels", getLabelsTest)
}

// buildImageTest runs before the other tests to build the "example/nginx-centos7", thus if
// successful other tests can rely in this image.
func buildImageTest(t *testing.T) {
	fs := fs.NewFileSystem()
	workingDir, err := fs.CreateWorkingDirectory()
	if err != nil {
		t.Fatalf("unable to create working directory: '%q'", err)
	}

	t.Logf("Copying into '%s'", workingDir)
	mkdirCmd := []string{"mkdir", "-p", path.Join(workingDir, "upload")}
	if _, err = buildahpkg.Execute(mkdirCmd, nil, true); err != nil {
		t.Fatalf("can't create temp directory: '%q'", err)
	}
	uploadDir := path.Join(workingDir, "upload/src")
	uploadCmd := []string{"cp", "-r", "../../../examples/nginx-centos7", uploadDir}
	if _, err = buildahpkg.Execute(uploadCmd, nil, true); err != nil {
		t.Fatalf("can't copy example directory on temporary workding dir: '%q'", err)
	}
	defer func() {
		_ = os.RemoveAll(workingDir)
	}()

	b := buildahpkg.NewBuildah(nil)
	opts := docker.BuildImageOptions{
		Name:       exampleImage,
		WorkingDir: workingDir,
	}

	if err = b.BuildImage(opts); err != nil {
		t.Fatalf("unable to build image: '%q'", err)
	}
}

func inspectImageTest(t *testing.T) {
	b := buildahpkg.NewBuildah(nil)
	imageMetadata, err := b.InspectImage(exampleImage)
	if err != nil {
		t.Logf("inspectImage error: '%q'", err)
		t.Fail() // expect no errors
	}
	if imageMetadata == nil {
		t.Fail() // expect metadata to be found
	} else {
		if imageMetadata.ID == "" {
			t.Fail() // expect metadata to carry image ID
		}
		if len(imageMetadata.Config.Labels) == 0 {
			t.Fail() // expect to find labels
		}
		if len(imageMetadata.Config.Env) == 0 {
			t.Fail() // expect to find environment variables
		}
	}
}

func isImageInLocalRegistryTest(t *testing.T) {
	b := buildahpkg.NewBuildah(nil)
	inLocalRegistry, _ := b.IsImageInLocalRegistry(exampleImage)
	if !inLocalRegistry {
		t.Logf("%s is not found on local registry, expect to be", exampleImage)
		t.Fail() // expect just built image to be in local registry
	}
}

func isImageOnBuildTest(t *testing.T) {
	b := buildahpkg.NewBuildah(nil)
	if b.IsImageOnBuild(busyboxImage) {
		t.Fail() // expect busybox image not be onbuild
	}
	if b.IsImageOnBuild(exampleImage) {
		t.Fail() // expect example image not be onbuild, as well
	}
}

func getOnBuildTest(t *testing.T) {
	b := buildahpkg.NewBuildah(nil)
	onBuild, err := b.GetOnBuild(exampleImage)
	if err != nil {
		t.Logf("GetOnBuild error: '%q'", err)
		t.Fail() // expect no errors on inspecting local images
	}
	if len(onBuild) != 0 {
		t.Logf("OnBuild: '%q'", onBuild)
		t.Fail() // expect onBuild to be an empty slice
	}
}

func getScriptsURLTest(t *testing.T) {
	b := buildahpkg.NewBuildah(nil)
	scriptsURL, err := b.GetScriptsURL(exampleImage)
	if err != nil {
		t.Logf("GetScriptURL error: '%q'", err)
		t.Fail() // expect no errors on inspecting local images
	}
	if scriptsURL == "" {
		t.Fail() // expect a value in scriptsURL
	}
}

func getAssembleInputFilesTest(t *testing.T) {
	b := buildahpkg.NewBuildah(nil)
	files, err := b.GetAssembleInputFiles(exampleImage)
	if err != nil {
		t.Logf("GetAssembleInputFiles error: '%q'", err)
		t.Fail() // expect no errors on inspecting local images
	}
	if files != "" {
		t.Fail() // expect empty string
	}
}

func getAssembleRuntimeUserTest(t *testing.T) {
	b := buildahpkg.NewBuildah(nil)
	user, err := b.GetAssembleRuntimeUser(exampleImage)
	if err != nil {
		t.Logf("GetAssembleRuntimeUser error: '%q'", err)
		t.Fail() // expect no  errors on inspecting local images
	}
	if user != "" {
		t.Fail() // expect empty user
	}
}

func getImageNameTest(t *testing.T) {
	name := buildahpkg.GetImageName(exampleImage)
	if name == "" {
		t.Fail() // expect to extract image name
	}
}

func getImageIDTest(t *testing.T) {
	b := buildahpkg.NewBuildah(nil)
	id, err := b.GetImageID(exampleImage)
	if err != nil {
		t.Logf("GetImageID error: '%q'", err)
		t.Fail() // expect no errors
	}
	if id == "" {
		t.Fail() // expect to be able do find image ID
	}
}

func getImageWorkdirTest(t *testing.T) {
	b := buildahpkg.NewBuildah(nil)
	workDir, err := b.GetImageWorkdir(exampleImage)
	if err != nil {
		t.Logf("GetImageWorkdir error: '%q'", err)
		t.Fail() // expect no errors
	}
	if workDir == "" {
		t.Fail() // expect workdir is set on example image
	}
}

func checkImageTest(t *testing.T) {
	b := buildahpkg.NewBuildah(nil)
	imageMetadata, err := b.CheckImage(exampleImage)
	if err != nil {
		t.Logf("CheckImage error: '%q'", err)
		t.Fail() // expect no errors
	}
	if imageMetadata == nil {
		t.Fail() // expect metadata about image
	}
}

func pullImageTest(t *testing.T) {
	b := buildahpkg.NewBuildah(nil)
	imageMetadata, err := b.PullImage(busyboxImage)
	if err != nil {
		t.Logf("PullImage error: '%q'", err)
		t.Fail() // expect no errors
	}
	if imageMetadata == nil {
		t.Fail() // expect to find image metadata
	}
}

func checkAndPullImageTest(t *testing.T) {
	b := buildahpkg.NewBuildah(nil)
	imageMetadata, err := b.CheckAndPullImage(busyboxImage)
	if err != nil {
		t.Logf("CheckAndPullImage error: '%q'", err)
		t.Fail() // expect no errors
	}
	if imageMetadata == nil {
		t.Fail() // expect to find image metadata
	}
}

func getImageUserTest(t *testing.T) {
	b := buildahpkg.NewBuildah(nil)
	user, err := b.GetImageUser(exampleImage)
	if err != nil {
		t.Logf("GetImageUser error: '%q'", err)
		t.Fail() // expect no errors
	}
	if user == "" {
		t.Fail() // expect to find image user
	}
}

func getImageEntrypointTest(t *testing.T) {
	b := buildahpkg.NewBuildah(nil)
	entrypoint, err := b.GetImageEntrypoint(exampleImage)
	if err != nil {
		t.Logf("GetImageEntrypoint error: '%q'", err)
		t.Fail() // expect no errors
	}
	if len(entrypoint) > 0 {
		t.Fail() // expect entrypoint not to be set
	}
}

func getLabelsTest(t *testing.T) {
	b := buildahpkg.NewBuildah(nil)
	labels, err := b.GetLabels(exampleImage)
	if err != nil {
		t.Logf("GetLabels error: '%q'", err)
		t.Fail() // expect no errors
	}
	if len(labels) == 0 {
		t.Fail() // expect to find labels on example image
	}
}

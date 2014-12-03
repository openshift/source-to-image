// +build integration,!no-docker

package integration

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"runtime"
	"testing"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/openshift/source-to-image/pkg/sti"
)

const (
	DefaultDockerSocket = "unix:///var/run/docker.sock"
	TestSource          = "git://github.com/pmorie/simple-html"

	FakeBaseImage        = "sti_test/sti-fake"
	FakeUserImage        = "sti_test/sti-fake-user"
	FakeBrokenBaseImage  = "sti_test/sti-fake-broken"
	FakeImageWithScripts = "sti_test/sti-fake-with-scripts"

	TagCleanBuild            = "test/sti-fake-app"
	TagCleanBuildUser        = "test/sti-fake-app-user"
	TagIncrementalBuild      = "test/sti-incremental-app"
	TagIncrementalBuildUser  = "test/sti-incremental-app-user"
	TagCleanBuildWithScripts = "test/sti-fake-app-with-scripts"

	// Need to serve the scripts from local host so any potential changes to the
	// scripts are made available for integration testing.
	//
	// Port 23456 must match the port used in the fake image Dockerfiles
	FakeScriptsHttpUrl = "http://127.0.0.1:23456/sti-fake/.sti/bin"
)

type integrationTest struct {
	t             *testing.T
	dockerClient  *docker.Client
	setupComplete bool
}

var (
	FakeScriptsFileUrl string
)

func dockerSocket() string {
	if dh := os.Getenv("DOCKER_HOST"); dh != "" {
		return dh
	}
	return DefaultDockerSocket
}

// setup sets up integration tests
func (i *integrationTest) setup() {
	i.dockerClient, _ = docker.NewClient(dockerSocket())
	if !i.setupComplete {
		// get the full path to this .go file so we can construct the file url
		// using this file's dirname
		_, filename, _, _ := runtime.Caller(0)
		testImagesDir := path.Join(path.Dir(filename), "images")
		FakeScriptsFileUrl = "file://" + path.Join(testImagesDir, "sti-fake", ".sti", "bin")

		for _, image := range []string{TagCleanBuild, TagCleanBuildUser, TagIncrementalBuild, TagIncrementalBuildUser} {
			i.dockerClient.RemoveImage(image)
		}

		go http.ListenAndServe(":23456", http.FileServer(http.Dir(testImagesDir)))
		i.t.Logf("Waiting for mock HTTP server to start...")
		if err := waitForHttpReady(); err != nil {
			i.t.Fatalf("Unexpected error: %v", err)
		}
		i.setupComplete = true
	}
}

func integration(t *testing.T) *integrationTest {
	i := &integrationTest{t: t}
	i.setup()
	return i
}

// Wait for the mock HTTP server to become ready to serve the HTTP requests.
//
func waitForHttpReady() error {
	retryCount := 50
	for {
		if resp, err := http.Get("http://127.0.0.1:23456/"); err != nil {
			resp.Body.Close()
			if retryCount -= 1; retryCount > 0 {
				time.Sleep(20 * time.Millisecond)
			} else {
				return err
			}
		} else {
			return nil
		}
	}
}

// Test a clean build.  The simplest case.
func TestCleanBuild(t *testing.T) {
	integration(t).exerciseCleanBuild(TagCleanBuild, false, FakeBaseImage, "")
}

func TestCleanBuildUser(t *testing.T) {
	integration(t).exerciseCleanBuild(TagCleanBuildUser, false, FakeUserImage, "")
}

func TestCleanBuildFileScriptsUrl(t *testing.T) {
	integration(t).exerciseCleanBuild(TagCleanBuild, false, FakeBaseImage, FakeScriptsFileUrl)
}

func TestCleanBuildHttpScriptsUrl(t *testing.T) {
	integration(t).exerciseCleanBuild(TagCleanBuild, false, FakeBaseImage, FakeScriptsHttpUrl)
}

func TestCleanBuildWithScripts(t *testing.T) {
	integration(t).exerciseCleanBuild(TagCleanBuildWithScripts, false, FakeImageWithScripts, "")
}

// Test that a build request with a callbackUrl will invoke HTTP endpoint
func TestCleanBuildCallbackInvoked(t *testing.T) {
	integration(t).exerciseCleanBuild(TagCleanBuild, true, FakeBaseImage, "")
}

func (i *integrationTest) exerciseCleanBuild(tag string, verifyCallback bool, imageName string, scriptsUrl string) {
	t := i.t
	callbackUrl := ""
	callbackInvoked := false
	callbackHasValidJson := false
	if verifyCallback {
		handler := func(w http.ResponseWriter, r *http.Request) {
			// we got called
			callbackInvoked = true
			// the header is as expected
			contentType := r.Header["Content-Type"][0]
			callbackHasValidJson = contentType == "application/json"
			// the request body is as expected
			if callbackHasValidJson {
				defer r.Body.Close()
				body, _ := ioutil.ReadAll(r.Body)
				type CallbackMessage struct {
					Payload string
					Success bool
				}
				var callbackMessage CallbackMessage
				err := json.Unmarshal(body, &callbackMessage)
				callbackHasValidJson = (err == nil) && (callbackMessage.Success)
			}
		}
		ts := httptest.NewServer(http.HandlerFunc(handler))
		defer ts.Close()
		callbackUrl = ts.URL
	}

	req := &sti.STIRequest{
		DockerSocket: dockerSocket(),
		BaseImage:    imageName,
		Source:       TestSource,
		Tag:          tag,
		Clean:        true,
		CallbackUrl:  callbackUrl,
		ScriptsUrl:   scriptsUrl}

	b, err := sti.NewBuilder(req)
	if err != nil {
		t.Errorf("Cannot create a new builder.")
		return
	}
	resp, err := b.Build()
	if err != nil {
		t.Errorf("An error occurred during the build: %v", err)
		return
	} else if !resp.Success {
		t.Errorf("The build failed.")
		return
	}
	if callbackInvoked != verifyCallback {
		t.Errorf("Sti build did not invoke callback")
		return
	}
	if callbackHasValidJson != verifyCallback {
		t.Errorf("Sti build did not invoke callback with valid json message")
		return
	}

	i.checkForImage(tag)
	containerId := i.createContainer(tag)
	defer i.removeContainer(containerId)
	i.checkBasicBuildState(containerId, resp.WorkingDir)
}

// Test an incremental build.
func TestIncrementalBuildAndRemovePreviousImage(t *testing.T) {
	integration(t).exerciseIncrementalBuild(TagIncrementalBuild, true)
}

func TestIncrementalBuildAndKeepPreviousImage(t *testing.T) {
	integration(t).exerciseIncrementalBuild(TagIncrementalBuild, false)
}

func TestIncrementalBuildUser(t *testing.T) {
	integration(t).exerciseIncrementalBuild(TagIncrementalBuildUser, true)
}

func (i *integrationTest) exerciseIncrementalBuild(tag string, removePreviousImage bool) {
	t := i.t
	req := &sti.STIRequest{
		DockerSocket:        dockerSocket(),
		BaseImage:           FakeBaseImage,
		Source:              TestSource,
		Tag:                 tag,
		Clean:               true,
		RemovePreviousImage: removePreviousImage,
	}

	builder, err := sti.NewBuilder(req)
	if err != nil {
		t.Errorf("Unable to create builder: %v", err)
		return
	}
	resp, err := builder.Build()
	if err != nil {
		t.Errorf("Unexpected error occurred during build: %v", err)
		return
	}
	if !resp.Success {
		t.Errorf("STI Build failed.")
		return
	}

	previousImageId := resp.ImageID

	req.Clean = false

	builder, err = sti.NewBuilder(req)
	if err != nil {
		t.Errorf("Unable to create incremental builder: %v", err)
		return
	}
	resp, err = builder.Build()
	if err != nil {
		t.Errorf("Unexpected error occurred during incremental build: %v", err)
		return
	}
	if !resp.Success {
		t.Errorf("STI incremental build failed.")
		return
	}

	i.checkForImage(tag)
	containerId := i.createContainer(tag)
	defer i.removeContainer(containerId)
	i.checkIncrementalBuildState(containerId, resp.WorkingDir)

	_, err = i.dockerClient.InspectImage(previousImageId)
	if removePreviousImage {
		if err == nil {
			t.Errorf("Previous image %s not deleted", previousImageId)
		}
	} else {
		if err != nil {
			t.Errorf("Coudln't find previous image %s", previousImageId)
		}
	}
}

// Support methods
func (i *integrationTest) checkForImage(tag string) {
	_, err := i.dockerClient.InspectImage(tag)
	if err != nil {
		i.t.Errorf("Couldn't find image with tag: %s", tag)
	}
}

func (i *integrationTest) createContainer(image string) string {
	config := docker.Config{Image: image, AttachStdout: false, AttachStdin: false}
	container, err := i.dockerClient.CreateContainer(docker.CreateContainerOptions{Name: "", Config: &config})
	if err != nil {
		i.t.Errorf("Couldn't create container from image %s", image)
		return ""
	}

	err = i.dockerClient.StartContainer(container.ID, &docker.HostConfig{})
	if err != nil {
		i.t.Errorf("Couldn't start container: %s", container.ID)
		return ""
	}

	exitCode, err := i.dockerClient.WaitContainer(container.ID)
	if exitCode != 0 {
		i.t.Errorf("Bad exit code from container: %d", exitCode)
		return ""
	}

	return container.ID
}

func (i *integrationTest) removeContainer(cId string) {
	i.dockerClient.RemoveContainer(docker.RemoveContainerOptions{cId, true, true})
}

func (i *integrationTest) checkFileExists(cId string, filePath string) {
	res := fileExistsInContainer(i.dockerClient, cId, filePath)

	if !res {
		i.t.Errorf("Couldn't find file %s in container %s", filePath, cId)
	}
}

func (i *integrationTest) checkBasicBuildState(cId string, workingDir string) {
	i.checkFileExists(cId, "/sti-fake/assemble-invoked")
	i.checkFileExists(cId, "/sti-fake/run-invoked")
	i.checkFileExists(cId, "/sti-fake/src/index.html")

	_, err := os.Stat(workingDir)
	if !os.IsNotExist(err) {
		i.t.Errorf("Unexpected error from stat check on %s", workingDir)
	}
}

func (i *integrationTest) checkIncrementalBuildState(cId string, workingDir string) {
	i.checkBasicBuildState(cId, workingDir)
	i.checkFileExists(cId, "/sti-fake/save-artifacts-invoked")
}

func fileExistsInContainer(d *docker.Client, cId string, filePath string) bool {
	var buf []byte
	writer := bytes.NewBuffer(buf)

	err := d.CopyFromContainer(docker.CopyFromContainerOptions{writer, cId, filePath})
	content := writer.String()

	return ((err == nil) && ("" != content))
}

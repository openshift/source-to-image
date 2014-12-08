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

	FakeBaseImage                   = "sti_test/sti-fake"
	FakeUserImage                   = "sti_test/sti-fake-user"
	FakeImageScripts                = "sti_test/sti-fake-scripts"
	FakeImageScriptsNoSaveArtifacts = "sti_test/sti-fake-scripts-no-save-artifacts"

	TagCleanBuild                             = "test/sti-fake-app"
	TagCleanBuildUser                         = "test/sti-fake-app-user"
	TagIncrementalBuild                       = "test/sti-incremental-app"
	TagIncrementalBuildUser                   = "test/sti-incremental-app-user"
	TagCleanBuildScripts                      = "test/sti-fake-app-scripts"
	TagIncrementalBuildScripts                = "test/sti-incremental-app-scripts"
	TagIncrementalBuildScriptsNoSaveArtifacts = "test/sti-incremental-app-scripts-no-save-artifacts"

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
	FakeScriptsFileURL string
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
		testImagesDir := path.Join(path.Dir(filename), "scripts")
		FakeScriptsFileURL = "file://" + path.Join(testImagesDir, ".sti", "bin")

		for _, image := range []string{TagCleanBuild, TagCleanBuildUser, TagIncrementalBuild, TagIncrementalBuildUser} {
			i.dockerClient.RemoveImage(image)
		}

		go http.ListenAndServe(":23456", http.FileServer(http.Dir(testImagesDir)))
		i.t.Logf("Waiting for mock HTTP server to start...")
		if err := waitForHTTPReady(); err != nil {
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
func waitForHTTPReady() error {
	retryCount := 50
	for {
		if resp, err := http.Get("http://127.0.0.1:23456/"); err != nil {
			resp.Body.Close()
			if retryCount--; retryCount > 0 {
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

func TestCleanBuildFileScriptsURL(t *testing.T) {
	integration(t).exerciseCleanBuild(TagCleanBuild, false, FakeBaseImage, FakeScriptsFileURL)
}

func TestCleanBuildHttpScriptsURL(t *testing.T) {
	integration(t).exerciseCleanBuild(TagCleanBuild, false, FakeBaseImage, FakeScriptsHttpUrl)
}

func TestCleanBuildScripts(t *testing.T) {
	integration(t).exerciseCleanBuild(TagCleanBuildScripts, false, FakeImageScripts, "")
}

// Test that a build request with a callbackURL will invoke HTTP endpoint
func TestCleanBuildCallbackInvoked(t *testing.T) {
	integration(t).exerciseCleanBuild(TagCleanBuild, true, FakeBaseImage, "")
}

func (i *integrationTest) exerciseCleanBuild(tag string, verifyCallback bool, imageName string, scriptsURL string) {
	t := i.t
	callbackURL := ""
	callbackInvoked := false
	callbackHasValidJSON := false
	if verifyCallback {
		handler := func(w http.ResponseWriter, r *http.Request) {
			// we got called
			callbackInvoked = true
			// the header is as expected
			contentType := r.Header["Content-Type"][0]
			callbackHasValidJSON = contentType == "application/json"
			// the request body is as expected
			if callbackHasValidJSON {
				defer r.Body.Close()
				body, _ := ioutil.ReadAll(r.Body)
				type CallbackMessage struct {
					Payload string
					Success bool
				}
				var callbackMessage CallbackMessage
				err := json.Unmarshal(body, &callbackMessage)
				callbackHasValidJSON = (err == nil) && (callbackMessage.Success)
			}
		}
		ts := httptest.NewServer(http.HandlerFunc(handler))
		defer ts.Close()
		callbackURL = ts.URL
	}

	req := &sti.Request{
		DockerSocket: dockerSocket(),
		BaseImage:    imageName,
		Source:       TestSource,
		Tag:          tag,
		Clean:        true,
		CallbackURL:  callbackURL,
		ScriptsURL:   scriptsURL}

	b, err := sti.NewBuilder(req)
	if err != nil {
		t.Fatalf("Cannot create a new builder.")
	}
	resp, err := b.Build()
	if err != nil {
		t.Fatalf("An error occurred during the build: %v", err)
	} else if !resp.Success {
		t.Fatalf("The build failed.")
	}
	if callbackInvoked != verifyCallback {
		t.Fatalf("Sti build did not invoke callback")
	}
	if callbackHasValidJSON != verifyCallback {
		t.Fatalf("Sti build did not invoke callback with valid json message")
	}

	i.checkForImage(tag)
	containerID := i.createContainer(tag)
	defer i.removeContainer(containerID)
	i.checkBasicBuildState(containerID, resp.WorkingDir)
}

// Test an incremental build.
func TestIncrementalBuildAndRemovePreviousImage(t *testing.T) {
	integration(t).exerciseIncrementalBuild(TagIncrementalBuild, FakeBaseImage, true, false)
}

func TestIncrementalBuildAndKeepPreviousImage(t *testing.T) {
	integration(t).exerciseIncrementalBuild(TagIncrementalBuild, FakeBaseImage, false, false)
}

func TestIncrementalBuildUser(t *testing.T) {
	integration(t).exerciseIncrementalBuild(TagIncrementalBuildUser, FakeBaseImage, true, false)
}

func TestIncrementalBuildScripts(t *testing.T) {
	integration(t).exerciseIncrementalBuild(TagIncrementalBuildScripts, FakeImageScripts, true, false)
}

func TestIncrementalBuildScriptsNoSaveArtifacts(t *testing.T) {
	integration(t).exerciseIncrementalBuild(TagIncrementalBuildScriptsNoSaveArtifacts, FakeImageScriptsNoSaveArtifacts, true, true)
}

func (i *integrationTest) exerciseIncrementalBuild(tag, imageName string, removePreviousImage bool, expectClean bool) {
	t := i.t
	req := &sti.Request{
		DockerSocket:        dockerSocket(),
		BaseImage:           imageName,
		Source:              TestSource,
		Tag:                 tag,
		Clean:               true,
		RemovePreviousImage: removePreviousImage,
	}

	builder, err := sti.NewBuilder(req)
	if err != nil {
		t.Fatalf("Unable to create builder: %v", err)
	}
	resp, err := builder.Build()
	if err != nil {
		t.Fatalf("Unexpected error occurred during build: %v", err)
	}
	if !resp.Success {
		t.Fatalf("STI Build failed.")
	}

	previousImageID := resp.ImageID

	req.Clean = false

	builder, err = sti.NewBuilder(req)
	if err != nil {
		t.Fatalf("Unable to create incremental builder: %v", err)
	}
	resp, err = builder.Build()
	if err != nil {
		t.Fatalf("Unexpected error occurred during incremental build: %v", err)
	}
	if !resp.Success {
		t.Fatalf("STI incremental build failed.")
	}

	i.checkForImage(tag)
	containerID := i.createContainer(tag)
	defer i.removeContainer(containerID)
	i.checkIncrementalBuildState(containerID, resp.WorkingDir, expectClean)

	_, err = i.dockerClient.InspectImage(previousImageID)
	if removePreviousImage {
		if err == nil {
			t.Errorf("Previous image %s not deleted", previousImageID)
		}
	} else {
		if err != nil {
			t.Errorf("Coudln't find previous image %s", previousImageID)
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

func (i *integrationTest) removeContainer(cID string) {
	i.dockerClient.RemoveContainer(docker.RemoveContainerOptions{cID, true, true})
}

func (i *integrationTest) fileExists(cID string, filePath string) {
	res := fileExistsInContainer(i.dockerClient, cID, filePath)

	if !res {
		i.t.Errorf("Couldn't find file %s in container %s", filePath, cID)
	}
}

func (i *integrationTest) fileNotExists(cID string, filePath string) {
	res := fileExistsInContainer(i.dockerClient, cID, filePath)

	if res {
		i.t.Errorf("Unexpected file %s in container %s", filePath, cID)
	}
}

func (i *integrationTest) checkBasicBuildState(cID string, workingDir string) {
	i.fileExists(cID, "/sti-fake/assemble-invoked")
	i.fileExists(cID, "/sti-fake/run-invoked")
	i.fileExists(cID, "/sti-fake/src/index.html")

	_, err := os.Stat(workingDir)
	if !os.IsNotExist(err) {
		i.t.Errorf("Unexpected error from stat check on %s", workingDir)
	}
}

func (i *integrationTest) checkIncrementalBuildState(cID string, workingDir string, expectClean bool) {
	i.checkBasicBuildState(cID, workingDir)
	if expectClean {
		i.fileNotExists(cID, "/sti-fake/save-artifacts-invoked")
	} else {
		i.fileExists(cID, "/sti-fake/save-artifacts-invoked")
	}
}

func fileExistsInContainer(d *docker.Client, cID string, filePath string) bool {
	var buf []byte
	writer := bytes.NewBuffer(buf)

	err := d.CopyFromContainer(docker.CopyFromContainerOptions{writer, cID, filePath})
	content := writer.String()

	return ((err == nil) && ("" != content))
}

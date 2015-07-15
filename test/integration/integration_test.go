// +build integration,!no-docker

package integration

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/golang/glog"
	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/build/strategies"
)

const (
	DefaultDockerSocket = "unix:///var/run/docker.sock"
	TestSource          = "git://github.com/pmorie/simple-html"

	FakeBuilderImage                = "sti_test/sti-fake"
	FakeUserImage                   = "sti_test/sti-fake-user"
	FakeImageScripts                = "sti_test/sti-fake-scripts"
	FakeImageScriptsNoSaveArtifacts = "sti_test/sti-fake-scripts-no-save-artifacts"
	FakeImageNoTar                  = "sti_test/sti-fake-no-tar"
	FakeImageOnBuild                = "sti_test/sti-fake-onbuild"

	TagCleanBuild                             = "test/sti-fake-app"
	TagCleanBuildUser                         = "test/sti-fake-app-user"
	TagIncrementalBuild                       = "test/sti-incremental-app"
	TagIncrementalBuildUser                   = "test/sti-incremental-app-user"
	TagCleanBuildScripts                      = "test/sti-fake-app-scripts"
	TagIncrementalBuildScripts                = "test/sti-incremental-app-scripts"
	TagIncrementalBuildScriptsNoSaveArtifacts = "test/sti-incremental-app-scripts-no-save-artifacts"
	TagCleanLayeredBuildNoTar                 = "test/sti-fake-no-tar"
	TagCleanBuildOnBuild                      = "test/sti-fake-app-onbuild"
	TagIncrementalBuildOnBuild                = "test/sti-incremental-app-onbuild"
	TagCleanBuildOnBuildNoName                = "test/sti-fake-app-onbuild-noname"
	TagCleanBuildNoName                       = "test/sti-fake-app-noname"
	TagCleanLayeredBuildNoTarNoName           = "test/sti-fake-no-tar-noname"

	// Need to serve the scripts from local host so any potential changes to the
	// scripts are made available for integration testing.
	//
	// Port 23456 must match the port used in the fake image Dockerfiles
	FakeScriptsHttpURL = "http://127.0.0.1:23456/sti-fake/.sti/bin"
)

type integrationTest struct {
	t             *testing.T
	dockerClient  *docker.Client
	setupComplete bool
}

var (
	FakeScriptsFileURL string
)

func dockerConfig() *api.DockerConfig {
	cfg := &api.DockerConfig{}
	if cfg.Endpoint = os.Getenv("DOCKER_HOST"); cfg.Endpoint == "" {
		cfg.Endpoint = DefaultDockerSocket
	}
	if os.Getenv("DOCKER_TLS_VERIFY") == "1" {
		certPath := os.Getenv("DOCKER_CERT_PATH")
		cfg.CertFile = filepath.Join(certPath, "cert.pem")
		cfg.KeyFile = filepath.Join(certPath, "key.pem")
		cfg.CAFile = filepath.Join(certPath, "ca.pem")
	}
	return cfg
}

func dockerClient(config *api.DockerConfig) (*docker.Client, error) {
	if config.CertFile != "" && config.KeyFile != "" && config.CAFile != "" {
		return docker.NewTLSClient(
			config.Endpoint,
			config.CertFile,
			config.KeyFile,
			config.CAFile)
	}
	return docker.NewClient(config.Endpoint)
}

func getLogLevel() (level int) {
	for level = 5; level >= 0; level-- {
		if glog.V(glog.Level(level)) == true {
			break
		}
	}
	return
}

// setup sets up integration tests
func (i *integrationTest) setup() {
	i.dockerClient, _ = dockerClient(dockerConfig())
	if !i.setupComplete {
		// get the full path to this .go file so we can construct the file url
		// using this file's dirname
		_, filename, _, _ := runtime.Caller(0)
		testImagesDir := filepath.Join(filepath.Dir(filename), "scripts")
		FakeScriptsFileURL = "file://" + filepath.Join(testImagesDir, ".sti", "bin")

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

	from := flag.CommandLine
	if vflag := from.Lookup("v"); vflag != nil {
		fmt.Fprintf(os.Stdout, "integration_test.go: found -v flag from starter bash script with initial glog level %d\n", getLogLevel())

		// the thing here is that we are looking for the bash -v passed into test-integration.sh (with no value),
		// but for glog (https://github.com/golang/glog/blob/master/glog.go), one specifies
		// the logging level with -v=# (i.e. -v=0 or -v=3 or -v=5).
		// so, for the changes stemming from issue 133, we 'reuse' the bash -v, and set the highest glog level.
		// (if you look at STI's main.go, and setupGlog, it essentially maps glog's -v to --loglevel for use by the sti command)
		//NOTE - passing --loglevel or -v=5 into test-integration.sh does not work
		if getLogLevel() != 5 {
			vflag.Value.Set("5")
			fmt.Fprintf(os.Stdout, "integration_test.go: glog level now %d\n", getLogLevel())
			// FIXME currently glog has only option to redirect output to stderr
			// the preferred for STI would be to redirect to stdout
			flag.CommandLine.Set("logtostderr", "true")
		}
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
	integration(t).exerciseCleanBuild(TagCleanBuild, false, FakeBuilderImage, "", true, true)
}

func TestCleanBuildUser(t *testing.T) {
	integration(t).exerciseCleanBuild(TagCleanBuildUser, false, FakeUserImage, "", true, true)
}

func TestCleanBuildFileScriptsURL(t *testing.T) {
	integration(t).exerciseCleanBuild(TagCleanBuild, false, FakeBuilderImage, FakeScriptsFileURL, true, true)
}

func TestCleanBuildHttpScriptsURL(t *testing.T) {
	integration(t).exerciseCleanBuild(TagCleanBuild, false, FakeBuilderImage, FakeScriptsHttpURL, true, true)
}

func TestCleanBuildScripts(t *testing.T) {
	integration(t).exerciseCleanBuild(TagCleanBuildScripts, false, FakeImageScripts, "", true, true)
}

func TestLayeredBuildNoTar(t *testing.T) {
	integration(t).exerciseCleanBuild(TagCleanLayeredBuildNoTar, false, FakeImageNoTar, FakeScriptsFileURL, false, true)
}

// Test that a build config with a callbackURL will invoke HTTP endpoint
func TestCleanBuildCallbackInvoked(t *testing.T) {
	integration(t).exerciseCleanBuild(TagCleanBuild, true, FakeBuilderImage, "", true, true)
}

func TestCleanBuildOnBuild(t *testing.T) {
	integration(t).exerciseCleanBuild(TagCleanBuildOnBuild, false, FakeImageOnBuild, "", true, true)
}

func TestCleanBuildOnBuildNoName(t *testing.T) {
	integration(t).exerciseCleanBuild(TagCleanBuildOnBuildNoName, false, FakeImageOnBuild, "", false, false)
}

func TestCleanBuildNoName(t *testing.T) {
	integration(t).exerciseCleanBuild(TagCleanBuildNoName, false, FakeBuilderImage, "", true, false)
}

func TestLayeredBuildNoTarNoName(t *testing.T) {
	integration(t).exerciseCleanBuild(TagCleanLayeredBuildNoTarNoName, false, FakeImageNoTar, FakeScriptsFileURL, false, false)
}

func (i *integrationTest) exerciseCleanBuild(tag string, verifyCallback bool, imageName string, scriptsURL string, expectImageName bool, setTag bool) {
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

	var buildTag string
	if setTag {
		buildTag = tag
	} else {
		buildTag = ""
	}

	config := &api.Config{
		DockerConfig: dockerConfig(),
		BuilderImage: imageName,
		Source:       TestSource,
		Tag:          buildTag,
		Incremental:  false,
		CallbackURL:  callbackURL,
		ScriptsURL:   scriptsURL}

	b, err := strategies.GetStrategy(config)
	if err != nil {
		t.Fatalf("Cannot create a new builder.")
	}
	resp, err := b.Build(config)
	if err != nil {
		t.Fatalf("An error occurred during the build: %v", err)
	} else if !resp.Success {
		t.Fatalf("The build failed.")
	}
	if callbackInvoked != verifyCallback {
		t.Fatalf("S2I build did not invoke callback")
	}
	if callbackHasValidJSON != verifyCallback {
		t.Fatalf("S2I build did not invoke callback with valid json message")
	}

	// We restrict this check to only when we are passing tag through the build config
	// since we will not end up with an available tag by that name from build
	if setTag {
		i.checkForImage(tag)
		containerID := i.createContainer(tag)
		defer i.removeContainer(containerID)
		i.checkBasicBuildState(containerID, resp.WorkingDir)
	}

	// Check if we receive back an ImageID when we are expecting to
	if expectImageName && len(resp.ImageID) == 0 {
		t.Fatalf("S2I build did not receive an ImageID in response")
	}
	if !expectImageName && len(resp.ImageID) > 0 {
		t.Fatalf("S2I build received an ImageID in response")
	}
}

// Test an incremental build.
func TestIncrementalBuildAndRemovePreviousImage(t *testing.T) {
	integration(t).exerciseIncrementalBuild(TagIncrementalBuild, FakeBuilderImage, true, false, false)
}

func TestIncrementalBuildAndKeepPreviousImage(t *testing.T) {
	integration(t).exerciseIncrementalBuild(TagIncrementalBuild, FakeBuilderImage, false, false, false)
}

func TestIncrementalBuildUser(t *testing.T) {
	integration(t).exerciseIncrementalBuild(TagIncrementalBuildUser, FakeBuilderImage, true, false, false)
}

func TestIncrementalBuildScripts(t *testing.T) {
	integration(t).exerciseIncrementalBuild(TagIncrementalBuildScripts, FakeImageScripts, true, false, false)
}

func TestIncrementalBuildScriptsNoSaveArtifacts(t *testing.T) {
	integration(t).exerciseIncrementalBuild(TagIncrementalBuildScriptsNoSaveArtifacts, FakeImageScriptsNoSaveArtifacts, true, true, false)
}

func TestIncrementalBuildOnBuild(t *testing.T) {
	integration(t).exerciseIncrementalBuild(TagIncrementalBuildOnBuild, FakeImageOnBuild, false, true, true)
}

func (i *integrationTest) exerciseIncrementalBuild(tag, imageName string, removePreviousImage bool, expectClean bool, checkOnBuild bool) {
	t := i.t
	config := &api.Config{
		DockerConfig:        dockerConfig(),
		BuilderImage:        imageName,
		Source:              TestSource,
		Tag:                 tag,
		Incremental:         false,
		RemovePreviousImage: removePreviousImage,
	}

	builder, err := strategies.GetStrategy(config)
	if err != nil {
		t.Fatalf("Unable to create builder: %v", err)
	}
	resp, err := builder.Build(config)
	if err != nil {
		t.Fatalf("Unexpected error occurred during build: %v", err)
	}
	if !resp.Success {
		t.Fatalf("S2I Build failed.")
	}

	previousImageID := resp.ImageID
	config = &api.Config{
		DockerConfig:        dockerConfig(),
		BuilderImage:        imageName,
		Source:              TestSource,
		Tag:                 tag,
		Incremental:         true,
		RemovePreviousImage: removePreviousImage,
	}

	builder, err = strategies.GetStrategy(config)
	if err != nil {
		t.Fatalf("Unable to create incremental builder: %v", err)
	}
	resp, err = builder.Build(config)
	if err != nil {
		t.Fatalf("Unexpected error occurred during incremental build: %v", err)
	}
	if !resp.Success {
		t.Fatalf("S2I incremental build failed.")
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

	if checkOnBuild {
		i.fileExists(containerID, "/sti-fake/src/onbuild")
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

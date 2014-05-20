package sti

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"

	. "launchpad.net/gocheck"

	"github.com/fsouza/go-dockerclient"
)

// Register gocheck with the 'testing' runner
func Test(t *testing.T) { TestingT(t) }

type IntegrationTestSuite struct {
	dockerClient *docker.Client
	tempDir      string
}

// Register IntegrationTestSuite with the gocheck suite manager and add support for 'go test' flags,
// viz: go test -integration
var (
	_ = Suite(&IntegrationTestSuite{})

	integration = flag.Bool("integration", false, "Include integration tests")
)

const (
	DockerSocket = "unix:///var/run/docker.sock"
	TestSource   = "git://github.com/pmorie/simple-html"

	FakeBaseImage       = "sti_test/sti-fake"
	FakeUserImage       = "sti_test/sti-fake-user"
	FakeBrokenBaseImage = "sti_test/sti-fake-broken"

	TagCleanBuild       = "test/sti-fake-app"
	TagIncrementalBuild = "test/sti-incremental-app"

	TagCleanBuildRun       = "test/sti-fake-app-run"
	TagCleanBuildRunUser   = "test/sti-fake-app-run-user"
	TagIncrementalBuildRun = "test/sti-incremental-app-run"
)

var (
	FakeScriptsUrl        = "file://" + path.Join(os.Getenv("STI_TEST_IMAGES_DIR"), "sti-fake", ".sti", "bin")
	FakeBrokenScriptsUrl  = "file://" + path.Join(os.Getenv("STI_TEST_IMAGES_DIR"), "sti-fake-broken", ".sti", "bin")
	FakeUserScriptsUrl    = "file://" + path.Join(os.Getenv("STI_TEST_IMAGES_DIR"), "sti-fake-user", ".sti", "bin")
)

// Suite/Test fixtures are provided by gocheck
func (s *IntegrationTestSuite) SetUpSuite(c *C) {
	if !*integration {
		c.Skip("-integration not provided")
	}

	s.dockerClient, _ = docker.NewClient(DockerSocket)
	for _, image := range []string{TagCleanBuild, TagIncrementalBuild} {
		s.dockerClient.RemoveImage(image)
		s.dockerClient.RemoveImage(image + "-run")
	}
}

func (s *IntegrationTestSuite) SetUpTest(c *C) {
	s.tempDir, _ = ioutil.TempDir("/tmp", "go-sti-integration")
}

// TestXxxx methods are identified as test cases

// Test a clean build.  The simplest case.
func (s *IntegrationTestSuite) TestCleanBuild(c *C) {
	s.exerciseCleanBuild(c, TagCleanBuild, false, FakeBaseImage, "")
}

func (s *IntegrationTestSuite) TestCleanBuildFileScriptsUrl(c *C) {
	s.exerciseCleanBuild(c, TagCleanBuild, false, FakeBaseImage, FakeScriptsUrl)
}

func (s *IntegrationTestSuite) TestCleanBuildRun(c *C) {
	s.exerciseCleanBuild(c, TagCleanBuildRun, false, FakeBaseImage, "")
}

func (s *IntegrationTestSuite) TestCleanBuildRunUser(c *C) {
	s.exerciseCleanBuild(c, TagCleanBuildRunUser, false, FakeUserImage, "")
}

// Test that a build request with a callbackUrl will invoke HTTP endpoint
func (s *IntegrationTestSuite) TestCleanBuildCallbackInvoked(c *C) {
	s.exerciseCleanBuild(c, TagCleanBuildRun, true, FakeBaseImage, "")
}

func (s *IntegrationTestSuite) exerciseCleanBuild(c *C, tag string, verifyCallback bool, imageName string, scriptsUrl string) {
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

	req := BuildRequest{
		Request: Request{
			WorkingDir:   s.tempDir,
			DockerSocket: DockerSocket,
			Verbose:      true,
			BaseImage:    imageName},
		Source:      TestSource,
		Tag:         tag,
		Clean:       true,
		Writer:      os.Stdout,
		CallbackUrl: callbackUrl,
		ScriptsUrl:  scriptsUrl}

	resp, err := Build(req)

	c.Assert(err, IsNil, Commentf("Sti build failed"))
	c.Assert(resp.Success, Equals, true, Commentf("Sti build failed"))
	c.Assert(callbackInvoked, Equals, verifyCallback, Commentf("Sti build did not invoke callback"))
	c.Assert(callbackHasValidJson, Equals, verifyCallback, Commentf("Sti build did not invoke callback with valid json message"))

	s.checkForImage(c, tag)
	containerId := s.createContainer(c, tag)
	defer s.removeContainer(containerId)
	s.checkBasicBuildState(c, containerId)
}

// Test an incremental build.
func (s *IntegrationTestSuite) TestIncrementalBuildRun(c *C) {
	s.exerciseIncrementalBuild(c, TagIncrementalBuildRun)
}

func (s *IntegrationTestSuite) exerciseIncrementalBuild(c *C, tag string) {
	req := BuildRequest{
		Request: Request{
			WorkingDir:   s.tempDir,
			DockerSocket: DockerSocket,
			Verbose:      true,
			BaseImage:    FakeBaseImage},
		Source: TestSource,
		Tag:    tag,
		Clean:  true,
		Writer: os.Stdout}

	resp, err := Build(req)
	c.Assert(err, IsNil, Commentf("Sti build failed"))
	c.Assert(resp.Success, Equals, true, Commentf("Sti build failed"))

	os.Remove(s.tempDir)
	s.tempDir, _ = ioutil.TempDir("", "go-sti-integration")
	req.WorkingDir = s.tempDir
	req.Clean = false

	resp, err = Build(req)
	c.Assert(err, IsNil, Commentf("Sti build failed"))
	c.Assert(resp.Success, Equals, true, Commentf("Sti build failed"))

	s.checkForImage(c, tag)
	containerId := s.createContainer(c, tag)
	defer s.removeContainer(containerId)
	s.checkIncrementalBuildState(c, containerId)
}

// Support methods
func (s *IntegrationTestSuite) checkForImage(c *C, tag string) {
	_, err := s.dockerClient.InspectImage(tag)
	c.Assert(err, IsNil, Commentf("Couldn't find built image"))
}

func (s *IntegrationTestSuite) createContainer(c *C, image string) string {
	config := docker.Config{Image: image, AttachStdout: false, AttachStdin: false}
	container, err := s.dockerClient.CreateContainer(docker.CreateContainerOptions{Name: "", Config: &config})
	c.Assert(err, IsNil, Commentf("Couldn't create container from image %s", image))

	err = s.dockerClient.StartContainer(container.ID, &docker.HostConfig{})
	c.Assert(err, IsNil, Commentf("Couldn't start container: %s", container.ID))

	exitCode, err := s.dockerClient.WaitContainer(container.ID)
	c.Assert(exitCode, Equals, 0, Commentf("Bad exit code from container: %d", exitCode))

	return container.ID
}

func (s *IntegrationTestSuite) removeContainer(cId string) {
	s.dockerClient.RemoveContainer(docker.RemoveContainerOptions{cId, true, true})
}

func (s *IntegrationTestSuite) checkFileExists(c *C, cId string, filePath string) {
	res := FileExistsInContainer(s.dockerClient, cId, filePath)

	c.Assert(res, Equals, true, Commentf("Couldn't find file %s in container %s", filePath, cId))
}

func (s *IntegrationTestSuite) checkBasicBuildState(c *C, cId string) {
	s.checkFileExists(c, cId, "/sti-fake/assemble-invoked")
	s.checkFileExists(c, cId, "/sti-fake/run-invoked")
	s.checkFileExists(c, cId, "/sti-fake/src/index.html")
}

func (s *IntegrationTestSuite) checkIncrementalBuildState(c *C, cId string) {
	s.checkBasicBuildState(c, cId)
	s.checkFileExists(c, cId, "/sti-fake/save-artifacts-invoked")
}

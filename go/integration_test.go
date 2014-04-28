package sti

import (
	"flag"
	"io/ioutil"
	"os"
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
// viz: go test -integration -extended
var (
	_ = Suite(&IntegrationTestSuite{})

	integration = flag.Bool("integration", false, "Include integration tests")
	extended    = flag.Bool("extended", false, "Include long-running tests")
)

const (
	DockerSocket = "unix:///var/run/docker.sock"
	TestSource   = "git://github.com/pmorie/simple-html"

	FakeBaseImage       = "pmorie/sti-fake"
	FakeBuildImage      = "pmorie/sti-fake-builder"
	FakeBrokenBaseImage = "pmorie/sti-fake-broken"

	TagCleanBuild               = "test/sti-fake-app"
	TagIncrementalBuild         = "test/sti-incremental-app"
	TagExtendedBuild            = "test/sti-extended-app"
	TagIncrementalExtendedBuild = "test/sti-inc-ext-app"

	TagCleanBuildRun               = "test/sti-fake-app-run"
	TagIncrementalBuildRun         = "test/sti-incremental-app-run"
	TagExtendedBuildRun            = "test/sti-extended-app-run"
	TagIncrementalExtendedBuildRun = "test/sti-inc-ext-app-run"
)

// Suite/Test fixtures are provided by gocheck
func (s *IntegrationTestSuite) SetUpSuite(c *C) {
	if !*integration {
		c.Skip("-integration not provided")
	}

	s.dockerClient, _ = docker.NewClient(DockerSocket)
	for _, image := range []string{TagCleanBuild, TagIncrementalBuild, TagExtendedBuild, TagIncrementalExtendedBuild} {
		s.dockerClient.RemoveImage(image)
		s.dockerClient.RemoveImage(image + "-run")
	}
}

func (s *IntegrationTestSuite) SetUpTest(c *C) {
	s.tempDir, _ = ioutil.TempDir("/tmp", "go-sti-integration")
}

// TestXxxx methods are identified as test cases

// Test the most basic validate case
func (s *IntegrationTestSuite) TestValidateSuccess(c *C) {
	req := ValidateRequest{
		Request: Request{
			WorkingDir:   s.tempDir,
			DockerSocket: DockerSocket,
			Debug:        true,
			BaseImage:    FakeBaseImage,
		},
		Incremental: false,
	}
	resp, err := Validate(req)
	c.Assert(err, IsNil, Commentf("Validation failed: err"))
	c.Assert(resp.Success, Equals, true, Commentf("Validation failed: invalid response"))
}

// Test a basic validation failure
func (s *IntegrationTestSuite) TestValidateFailure(c *C) {
	req := ValidateRequest{
		Request: Request{
			WorkingDir:   s.tempDir,
			DockerSocket: DockerSocket,
			Debug:        true,
			BaseImage:    FakeBrokenBaseImage,
		},
		Incremental: false,
	}
	resp, err := Validate(req)
	c.Assert(err, IsNil, Commentf("Validation failed: err"))
	c.Assert(resp.Success, Equals, false, Commentf("Validation should have failed: invalid response"))
}

// Test an extended validation
func (s *IntegrationTestSuite) TestValidateExtendedSuccess(c *C) {
	req := ValidateRequest{
		Request: Request{
			WorkingDir:   s.tempDir,
			DockerSocket: DockerSocket,
			Debug:        true,
			BaseImage:    FakeBaseImage,
			RuntimeImage: FakeBaseImage,
		},
	}
	resp, err := Validate(req)
	c.Assert(err, IsNil, Commentf("Validation failed: err"))
	c.Assert(resp.Success, Equals, true, Commentf("Validation failed: invalid response"))
}

// Test an extended validation with a broken runtime image
func (s *IntegrationTestSuite) TestValidateExtendedFailure(c *C) {
	req := ValidateRequest{
		Request: Request{
			WorkingDir:   s.tempDir,
			DockerSocket: DockerSocket,
			Debug:        true,
			BaseImage:    FakeBaseImage,
			RuntimeImage: FakeBrokenBaseImage,
		},
	}
	resp, err := Validate(req)
	c.Assert(err, IsNil, Commentf("Validation failed: err"))
	c.Assert(resp.Success, Equals, false, Commentf("Validation should have failed: invalid response"))
}

// Test a clean build.  The simplest case.
func (s *IntegrationTestSuite) TestCleanBuild(c *C) {
	s.exerciseCleanBuild(c, TagCleanBuild, false)
}

func (s *IntegrationTestSuite) TestCleanBuildRun(c *C) {
	s.exerciseCleanBuild(c, TagCleanBuildRun, true)
}

func (s *IntegrationTestSuite) exerciseCleanBuild(c *C, tag string, useRun bool) {
	req := BuildRequest{
		Request: Request{
			WorkingDir:   s.tempDir,
			DockerSocket: DockerSocket,
			Debug:        true,
			BaseImage:    FakeBaseImage},
		Source: TestSource,
		Tag:    tag,
		Clean:  true,
		Writer: os.Stdout}

	if useRun {
		req.Method = "run"
	} else {
		req.Method = "build"
	}

	resp, err := Build(req)
	c.Assert(err, IsNil, Commentf("Sti build failed"))
	c.Assert(resp.Success, Equals, true, Commentf("Sti build failed"))

	s.checkForImage(c, tag)
	containerId := s.createContainer(c, tag)
	defer s.removeContainer(containerId)
	s.checkBasicBuildState(c, containerId)
}

// Test an incremental build.
func (s *IntegrationTestSuite) TestIncrementalBuild(c *C) {
	s.exerciseIncrementalBuild(c, TagIncrementalBuild, false)
}

func (s *IntegrationTestSuite) TestIncrementalBuildRun(c *C) {
	s.exerciseIncrementalBuild(c, TagIncrementalBuildRun, true)
}

func (s *IntegrationTestSuite) exerciseIncrementalBuild(c *C, tag string, useRun bool) {
	if !*extended {
		c.Skip("-extended not provided")
	}

	req := BuildRequest{
		Request: Request{
			WorkingDir:   s.tempDir,
			DockerSocket: DockerSocket,
			Debug:        true,
			BaseImage:    FakeBaseImage},
		Source: TestSource,
		Tag:    tag,
		Clean:  true,
		Writer: os.Stdout}

	if useRun {
		req.Method = "run"
	} else {
		req.Method = "build"
	}

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

// Test an extended build.
func (s *IntegrationTestSuite) TestCleanExtendedBuild(c *C) {
	s.exerciseCleanExtendedBuild(c, TagExtendedBuild, false)
}

func (s *IntegrationTestSuite) TestCleanExtendedBuildRun(c *C) {
	s.exerciseCleanExtendedBuild(c, TagExtendedBuildRun, true)
}

func (s *IntegrationTestSuite) exerciseCleanExtendedBuild(c *C, tag string, useRun bool) {
	req := BuildRequest{
		Request: Request{
			WorkingDir:   s.tempDir,
			DockerSocket: DockerSocket,
			Debug:        true,
			BaseImage:    FakeBuildImage,
			RuntimeImage: FakeBaseImage},
		Source: TestSource,
		Tag:    tag,
		Clean:  true,
		Writer: os.Stdout}

	if useRun {
		req.Method = "run"
	} else {
		req.Method = "build"
	}

	resp, err := Build(req)
	c.Assert(err, IsNil, Commentf("Sti build failed"))
	c.Assert(resp.Success, Equals, true, Commentf("Sti build failed"))

	s.checkForImage(c, tag)
	containerId := s.createContainer(c, tag)
	defer s.removeContainer(containerId)
	s.checkExtendedBuildState(c, containerId)
}

// Test an incremental extended build
func (s *IntegrationTestSuite) TestIncrementalExtendedBuild(c *C) {
	s.exerciseIncrementalExtendedBuild(c, TagIncrementalExtendedBuild, false)
}

func (s *IntegrationTestSuite) TestIncrementalExtendedBuildRun(c *C) {
	s.exerciseIncrementalExtendedBuild(c, TagIncrementalExtendedBuildRun, false)
}

func (s *IntegrationTestSuite) exerciseIncrementalExtendedBuild(c *C, tag string, useRun bool) {
	if !*extended {
		c.Skip("-extended not provided")
	}

	req := BuildRequest{
		Request: Request{
			WorkingDir:   s.tempDir,
			DockerSocket: DockerSocket,
			Debug:        true,
			BaseImage:    FakeBuildImage,
			RuntimeImage: FakeBaseImage},
		Source: TestSource,
		Tag:    tag,
		Clean:  true,
		Writer: os.Stdout}

	if useRun {
		req.Method = "run"
	} else {
		req.Method = "build"
	}

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
	s.checkIncrementalExtendedBuildState(c, containerId)
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
	s.checkFileExists(c, cId, "/sti-fake/prepare-invoked")
	s.checkFileExists(c, cId, "/sti-fake/run-invoked")
	s.checkFileExists(c, cId, "/sti-fake/src/index.html")
}

func (s *IntegrationTestSuite) checkIncrementalBuildState(c *C, cId string) {
	s.checkBasicBuildState(c, cId)
	s.checkFileExists(c, cId, "/sti-fake/save-artifacts-invoked")
}

func (s *IntegrationTestSuite) checkExtendedBuildState(c *C, cId string) {
	s.checkFileExists(c, cId, "/sti-fake/prepare-invoked")
	s.checkFileExists(c, cId, "/sti-fake/run-invoked")
}

func (s *IntegrationTestSuite) checkIncrementalExtendedBuildState(c *C, cId string) {
	s.checkExtendedBuildState(c, cId)
	s.checkFileExists(c, cId, "/sti-fake/src/save-artifacts-invoked")
}

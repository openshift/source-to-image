// +build integration,!no-docker

package integration

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	dockerapi "github.com/docker/engine-api/client"
	dockertypes "github.com/docker/engine-api/types"
	dockercontainer "github.com/docker/engine-api/types/container"
	dockerstrslice "github.com/docker/engine-api/types/strslice"
	"github.com/docker/go-connections/tlsconfig"
	"github.com/golang/glog"
	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/build/strategies"
	"github.com/openshift/source-to-image/pkg/util"
	"golang.org/x/net/context"
	"k8s.io/kubernetes/pkg/kubelet/dockertools"
	k8snet "k8s.io/kubernetes/pkg/util/net"
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
	FakeNumericUserImage            = "sti_test/sti-fake-numericuser"
	FakeImageOnBuildRootUser        = "sti_test/sti-fake-onbuild-rootuser"
	FakeImageOnBuildNumericUser     = "sti_test/sti-fake-onbuild-numericuser"

	TagCleanBuild                              = "test/sti-fake-app"
	TagCleanBuildUser                          = "test/sti-fake-app-user"
	TagIncrementalBuild                        = "test/sti-incremental-app"
	TagIncrementalBuildUser                    = "test/sti-incremental-app-user"
	TagCleanBuildScripts                       = "test/sti-fake-app-scripts"
	TagIncrementalBuildScripts                 = "test/sti-incremental-app-scripts"
	TagIncrementalBuildScriptsNoSaveArtifacts  = "test/sti-incremental-app-scripts-no-save-artifacts"
	TagCleanLayeredBuildNoTar                  = "test/sti-fake-no-tar"
	TagCleanBuildOnBuild                       = "test/sti-fake-app-onbuild"
	TagIncrementalBuildOnBuild                 = "test/sti-incremental-app-onbuild"
	TagCleanBuildOnBuildNoName                 = "test/sti-fake-app-onbuild-noname"
	TagCleanBuildNoName                        = "test/sti-fake-app-noname"
	TagCleanLayeredBuildNoTarNoName            = "test/sti-fake-no-tar-noname"
	TagCleanBuildAllowedUIDsNamedUser          = "test/sti-fake-alloweduids-nameduser"
	TagCleanBuildAllowedUIDsNumericUser        = "test/sti-fake-alloweduids-numericuser"
	TagCleanBuildAllowedUIDsOnBuildRoot        = "test/sti-fake-alloweduids-onbuildroot"
	TagCleanBuildAllowedUIDsOnBuildNumericUser = "test/sti-fake-alloweduids-onbuildnumeric"

	// Need to serve the scripts from local host so any potential changes to the
	// scripts are made available for integration testing.
	//
	// Port 23456 must match the port used in the fake image Dockerfiles
	FakeScriptsHttpURL = "http://127.0.0.1:23456/.s2i/bin"
)

// TestInjectionBuild tests the build where we inject files to assemble script.
func TestInjectionBuild(t *testing.T) {
	integration(t).exerciseInjectionBuild(TagCleanBuild, FakeBuilderImage, []string{
		"/tmp/s2i-test-dir:/tmp",
		"/tmp/s2i-test-dir:",
	})
}

type integrationTest struct {
	t             *testing.T
	dockerClient  dockertools.DockerInterface
	engineClient  dockerapi.Client
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

func dockerClient(config *api.DockerConfig) (dockertools.DockerInterface, dockerapi.Client, error) {
	var client *dockerapi.Client
	var httpClient *http.Client
	if config.CertFile != "" && config.KeyFile != "" && config.CAFile != "" {
		tlscOptions := tlsconfig.Options{
			CAFile:   config.CAFile,
			CertFile: config.CertFile,
			KeyFile:  config.KeyFile,
		}
		tlsc, tlsErr := tlsconfig.Client(tlscOptions)
		if tlsErr != nil {
			return nil, dockerapi.Client{}, tlsErr
		}
		httpClient = &http.Client{
			Transport: k8snet.SetTransportDefaults(&http.Transport{
				TLSClientConfig: tlsc,
			}),
		}
	}

	client, err := dockerapi.NewClient(config.Endpoint, "", httpClient, nil)
	if err != nil {
		return nil, dockerapi.Client{}, err
	}
	k8sDocker := dockertools.ConnectToDockerOrDie(config.Endpoint)
	return k8sDocker, *client, nil
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
	var err error
	i.dockerClient, i.engineClient, err = dockerClient(dockerConfig())
	if err != nil {
		i.t.Errorf("%+v", err)
		return
	}
	if !i.setupComplete {
		// get the full path to this .go file so we can construct the file url
		// using this file's dirname
		_, filename, _, _ := runtime.Caller(0)
		testImagesDir := filepath.Join(filepath.Dir(filename), "scripts")
		FakeScriptsFileURL = "file://" + filepath.Join(testImagesDir, ".s2i", "bin")

		for _, image := range []string{TagCleanBuild, TagCleanBuildUser, TagIncrementalBuild, TagIncrementalBuildUser} {
			i.dockerClient.RemoveImage(image, dockertypes.ImageRemoveOptions{})
		}

		go http.ListenAndServe(":23456", http.FileServer(http.Dir(testImagesDir)))
		if err := waitForHTTPReady(); err != nil {
			i.t.Fatalf("Unexpected error: %v", err)
		}
		i.setupComplete = true
	}

	from := flag.CommandLine
	if vflag := from.Lookup("v"); vflag != nil {
		// the thing here is that we are looking for the bash -v passed into test-integration.sh (with no value),
		// but for glog (https://github.com/golang/glog/blob/master/glog.go), one specifies
		// the logging level with -v=# (i.e. -v=0 or -v=3 or -v=5).
		// so, for the changes stemming from issue 133, we 'reuse' the bash -v, and set the highest glog level.
		// (if you look at STI's main.go, and setupGlog, it essentially maps glog's -v to --loglevel for use by the sti command)
		//NOTE - passing --loglevel or -v=5 into test-integration.sh does not work
		if getLogLevel() != 5 {
			vflag.Value.Set("5")
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
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}
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

func TestAllowedUIDsNamedUser(t *testing.T) {
	integration(t).exerciseCleanAllowedUIDsBuild(TagCleanBuildAllowedUIDsNamedUser, FakeUserImage, true)
}

func TestAllowedUIDsNumericUser(t *testing.T) {
	integration(t).exerciseCleanAllowedUIDsBuild(TagCleanBuildAllowedUIDsNumericUser, FakeNumericUserImage, false)
}

func TestAllowedUIDsOnBuildRootUser(t *testing.T) {
	integration(t).exerciseCleanAllowedUIDsBuild(TagCleanBuildAllowedUIDsNamedUser, FakeImageOnBuildRootUser, true)
}

func TestAllowedUIDsOnBuildNumericUser(t *testing.T) {
	integration(t).exerciseCleanAllowedUIDsBuild(TagCleanBuildAllowedUIDsNumericUser, FakeImageOnBuildNumericUser, false)
}

func (i *integrationTest) exerciseCleanAllowedUIDsBuild(tag, imageName string, expectError bool) {
	t := i.t
	config := &api.Config{
		DockerConfig:      dockerConfig(),
		BuilderImage:      imageName,
		BuilderPullPolicy: api.DefaultBuilderPullPolicy,
		Source:            TestSource,
		Tag:               tag,
		Incremental:       false,
		ScriptsURL:        "",
	}
	config.AllowedUIDs.Set("1-")
	_, err := strategies.GetStrategy(config)
	if err != nil && !expectError {
		t.Fatalf("Cannot create a new builder: %v", err)
	}
	if err == nil && expectError {
		t.Fatalf("Did not get an error and was expecting one.")
	}
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
		DockerConfig:      dockerConfig(),
		BuilderImage:      imageName,
		BuilderPullPolicy: api.DefaultBuilderPullPolicy,
		Source:            TestSource,
		Tag:               buildTag,
		Incremental:       false,
		CallbackURL:       callbackURL,
		ScriptsURL:        scriptsURL}

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

func (i *integrationTest) exerciseInjectionBuild(tag, imageName string, injections []string) {
	t := i.t
	err := os.Mkdir("/tmp/s2i-test-dir", 0777)
	if err != nil {
		t.Errorf("Unable to create temporary directory: %v", err)
	}
	defer os.RemoveAll("/tmp/s2i-test-dir")
	err = ioutil.WriteFile(filepath.Join("/tmp/s2i-test-dir/secret"), []byte("secret"), 0666)
	if err != nil {
		t.Errorf("Unable to write content to temporary injection file: %v", err)
	}
	injectionList := api.VolumeList{}
	for _, i := range injections {
		injectionList.Set(i)
	}
	config := &api.Config{
		DockerConfig:      dockerConfig(),
		BuilderImage:      imageName,
		BuilderPullPolicy: api.DefaultBuilderPullPolicy,
		Source:            TestSource,
		Tag:               tag,
		Injections:        injectionList,
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
		t.Fatalf("S2I build failed.")
	}
	i.checkForImage(tag)
	containerID := i.createContainer(tag)
	defer i.removeContainer(containerID)

	// Check that the injected file is delivered to assemble script
	i.fileExists(containerID, "/sti-fake/secret-delivered")
	i.fileExists(containerID, "/sti-fake/relative-secret-delivered")

	// Make sure the injected file does not exists in resulting image
	files, err := util.ExpandInjectedFiles(injectionList)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	for _, f := range files {
		if exitCode := i.runInImage(tag, "test -s "+f); exitCode == 0 {
			t.Errorf("The file must be empty: %q, we got %q", f, err)
		}
	}
}

func (i *integrationTest) exerciseIncrementalBuild(tag, imageName string, removePreviousImage bool, expectClean bool, checkOnBuild bool) {
	t := i.t
	config := &api.Config{
		DockerConfig:        dockerConfig(),
		BuilderImage:        imageName,
		BuilderPullPolicy:   api.DefaultBuilderPullPolicy,
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
		t.Fatalf("S2I build failed.")
	}

	previousImageID := resp.ImageID
	config = &api.Config{
		DockerConfig:            dockerConfig(),
		BuilderImage:            imageName,
		BuilderPullPolicy:       api.DefaultBuilderPullPolicy,
		Source:                  TestSource,
		Tag:                     tag,
		Incremental:             true,
		RemovePreviousImage:     removePreviousImage,
		PreviousImagePullPolicy: api.PullIfNotPresent,
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
			t.Errorf("Couldn't find previous image %s", previousImageID)
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
	opts := dockertypes.ContainerCreateConfig{Name: "", Config: &dockercontainer.Config{Image: image}}
	container, err := i.dockerClient.CreateContainer(opts)
	if err != nil {
		i.t.Errorf("Couldn't create container from image %s with error %+v", image, err)
		return ""
	}

	err = i.dockerClient.StartContainer(container.ID)
	if err != nil {
		i.t.Errorf("Couldn't start container: %s with error %+v", container.ID, err)
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	exitCode, err := i.engineClient.ContainerWait(ctx, container.ID)
	if exitCode != 0 {
		i.t.Errorf("Bad exit code from container: %d", exitCode)
		return ""
	}

	return container.ID
}

func (i *integrationTest) runInContainer(image string, command []string) int {
	opts := dockertypes.ContainerCreateConfig{Name: "", Config: &dockercontainer.Config{Image: image, AttachStdout: false, AttachStdin: false, Cmd: dockerstrslice.StrSlice(command)}}
	container, err := i.dockerClient.CreateContainer(opts)
	if err != nil || container == nil {
		i.t.Errorf("Couldn't create container from image %s err %+v", image, err)
		return -1
	}

	err = i.dockerClient.StartContainer(container.ID)
	if err != nil {
		i.t.Errorf("Couldn't start container: %s", container.ID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	exitCode, err := i.engineClient.ContainerWait(ctx, container.ID)
	if err != nil {
		i.t.Errorf("Couldn't start container: %s", container.ID)
	}
	return exitCode
}

func (i *integrationTest) removeContainer(cID string) {
	i.dockerClient.RemoveContainer(cID, dockertypes.ContainerRemoveOptions{true, true, true})
}

func (i *integrationTest) fileExists(cID string, filePath string) {
	res := i.fileExistsInContainer(cID, filePath)

	if !res {
		i.t.Errorf("Couldn't find file %s in container %s", filePath, cID)
	}
}

func (i *integrationTest) fileNotExists(cID string, filePath string) {
	res := i.fileExistsInContainer(cID, filePath)

	if res {
		i.t.Errorf("Unexpected file %s in container %s", filePath, cID)
	}
}

func (i *integrationTest) runInImage(image string, cmd string) int {
	return i.runInContainer(image, []string{"/bin/sh", "-c", cmd})
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

func (i *integrationTest) fileExistsInContainer(cID string, filePath string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	rdr, stats, err := i.engineClient.CopyFromContainer(ctx, cID, filePath)
	if err != nil {
		return false
	}
	defer rdr.Close()
	return "" != stats.Name
}

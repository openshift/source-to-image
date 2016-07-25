package docker

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/docker/test"

	dockertypes "github.com/docker/engine-api/types"
	dockercontainer "github.com/docker/engine-api/types/container"
	dockerstrslice "github.com/docker/engine-api/types/strslice"
	"k8s.io/kubernetes/pkg/kubelet/dockertools"
)

func TestContainerName(t *testing.T) {
	rand.Seed(0)
	got := containerName("sub.domain.com:5000/repo:tag@sha256:ffffff")
	want := "s2i_sub_domain_com_5000_repo_tag_sha256_ffffff_f1f85ff5"
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func getDocker(client Client) *stiDocker {
	//k8s has its own fake docker client mechanism
	k8sDocker := dockertools.ConnectToDockerOrDie("fake://")
	return &stiDocker{
		kubeDockerClient: k8sDocker,
		client:           client,
		pullAuth:         dockertypes.AuthConfig{},
	}
}

//NOTE, neither k8s kube client nor engine api make their error types public, so we cannot access / instantiate them like we did for the analogous
// ones in go-dockerclient.
// Hence, we test with errors created in these tests and set on the fake client  vs. those actually defined in k8s/engine-api.
// Also, the fake client does not save the parameters passed in; but instead, you can confirm that it was called via AssertCalls

func TestIsImageInLocalRegistry(t *testing.T) {
	type testDef struct {
		imageName      string
		docker         test.FakeDockerClient
		expectedResult bool
		expectedError  string
	}
	tests := map[string]testDef{
		"ImageFound":    {"a_test_image", test.FakeDockerClient{}, true, ""},
		"ImageNotFound": {"a_test_image:sometag", test.FakeDockerClient{}, false, "unable to get metadata for a_test_image:sometag"},
	}

	for test, def := range tests {
		dh := getDocker(&def.docker)
		fake := dh.kubeDockerClient.(*dockertools.FakeDockerClient)
		if def.expectedResult {
			fake.Image = &dockertypes.ImageInspect{ID: def.imageName}
		}

		result, err := dh.IsImageInLocalRegistry(def.imageName)

		if e := fake.AssertCalls([]string{"inspect_image"}); e != nil {
			t.Errorf("%+v", e)
		}

		if result != def.expectedResult {
			t.Errorf("Test - %s: Expected result: %v. Got: %v", test, def.expectedResult, result)
		}
		if err != nil && len(def.expectedError) > 0 && !strings.Contains(err.Error(), def.expectedError) {
			t.Errorf("Test - %s: Expected error: Got: %+v", test, err)
		}
	}
}
func TestCheckAndPullImage(t *testing.T) {
	// in addition to the deltas mentioned up top, kube fake client can't distinguish between exists and successful pull, and
	// pull options with engine-api don't reference repo name like go-dockerclient did ... removed tests related to that
	type testDef struct {
		imageName     string
		docker        test.FakeDockerClient
		calls         []string
		errorStage    string
		expectedError string
		maskedError   string
	}
	imageExistsTest := testDef{
		imageName: "test_image",
		docker:    test.FakeDockerClient{},
		calls:     []string{"inspect_image"},
	}
	imageDoesNotExistsTest := testDef{
		imageName:   "test_image",
		docker:      test.FakeDockerClient{},
		calls:       []string{"inspect_image", "pull", "inspect_image"},
		errorStage:  "inspect_image",
		maskedError: "no such image",
	}
	inspectErrorTest := testDef{
		imageName:     "test_image",
		docker:        test.FakeDockerClient{},
		calls:         []string{"inspect_image"},
		errorStage:    "inspect_image",
		expectedError: "unable to get metadata for test_image:latest",
	}
	pullErrorTest := testDef{
		imageName:     "test_image",
		docker:        test.FakeDockerClient{},
		calls:         []string{"inspect_image", "pull"},
		errorStage:    "pull",
		expectedError: "unable to get test_image:latest",
	}
	tests := map[string]testDef{
		"ImageExists":       imageExistsTest,
		"ImageDoesNotExist": imageDoesNotExistsTest,
		"InspectError":      inspectErrorTest,
		"PullError":         pullErrorTest,
	}

	for test, def := range tests {
		dh := getDocker(&def.docker)
		fake := dh.kubeDockerClient.(*dockertools.FakeDockerClient)
		if len(def.maskedError) > 0 {
			fake.ClearErrors()
			fake.InjectError(def.errorStage, fmt.Errorf(def.maskedError))
			fake.Image = nil
		} else if len(def.expectedError) > 0 {
			fake.ClearErrors()
			fake.InjectError(def.errorStage, fmt.Errorf(def.expectedError))
			fake.Image = nil
		} else {
			fake.Image = &dockertypes.ImageInspect{ID: def.imageName}
		}

		resultImage, resultErr := dh.CheckAndPullImage(def.imageName)

		if e := fake.AssertCalls(def.calls); e != nil {
			t.Errorf("%s %+v", test, e)
		}

		if len(def.maskedError) > 0 && resultErr != nil {
			t.Errorf("%s: Unexpected error -- %v", test, resultErr)
		}

		if len(def.expectedError) > 0 && (resultErr == nil || resultErr.Error() != def.expectedError) {
			t.Errorf("%s: Unexpected error result -- %v", test, resultErr)
		}
		if fake.Image != nil && (resultImage == nil || resultImage.ID != def.imageName) {
			t.Errorf("%s: Unexpected image result -- %+v instead of %+v", test, resultImage, fake.Image)
		}
	}
}

func TestRemoveContainer(t *testing.T) {
	fakeDocker := &test.FakeDockerClient{}
	dh := getDocker(fakeDocker)
	fake := dh.kubeDockerClient.(*dockertools.FakeDockerClient)
	containerID := "testContainerId"
	fakeContainer := dockertools.FakeContainer{ID: containerID}
	fake.SetFakeContainers([]*dockertools.FakeContainer{&fakeContainer})
	err := dh.RemoveContainer(containerID)
	if err != nil {
		t.Errorf("%+v", err)
	}
	err = fake.AssertCalls([]string{"remove"})
	if err != nil {
		t.Errorf("%v+v", err)
	}
}

func TestCommitContainer(t *testing.T) {
	type commit_test struct {
		containerID     string
		containerTag    string
		expectedImageID string
		expectedError   error
	}

	tests := map[string]commit_test{
		"valid": {
			containerID:     "test-container-id",
			containerTag:    "test-container-tag",
			expectedImageID: "test-container-tag",
		},
		"error": {
			containerID:     "test-container-id",
			containerTag:    "test-container-tag",
			expectedImageID: "test-container-tag",
			expectedError:   fmt.Errorf("Test error"),
		},
	}

	for desc, tst := range tests {
		opt := CommitContainerOptions{
			ContainerID: tst.containerID,
			Repository:  tst.containerTag,
		}
		param := dockertypes.ContainerCommitOptions{
			Reference: tst.containerTag,
		}
		resp := dockertypes.ContainerCommitResponse{
			ID: tst.expectedImageID,
		}
		fakeDocker := &test.FakeDockerClient{
			ContainerCommitResponse: resp,
			ContainerCommitErr:      tst.expectedError,
		}
		dh := getDocker(fakeDocker)

		imageID, err := dh.CommitContainer(opt)
		if err != tst.expectedError {
			t.Errorf("test case %s: Unexpected error returned: %v", desc, err)
		}
		if tst.containerID != fakeDocker.ContainerCommitID {
			t.Errorf("test case %s: Commit container called with unexpected container id: %s and %+v", desc, tst.containerID, fakeDocker.ContainerCommitID)
		}
		if !reflect.DeepEqual(param, fakeDocker.ContainerCommitOptions) {
			t.Errorf("test case %s: Commit container called with unexpected parameters: %+v and %+v", desc, param, fakeDocker.ContainerCommitOptions)
		}
		if tst.expectedError == nil && imageID != tst.expectedImageID {
			t.Errorf("test case %s: Did not return the correct image id: %s", desc, imageID)
		}
	}
}

func TestCopyToContainer(t *testing.T) {
	type copyto_test struct {
		containerID string
		src         string
		destPath    string
	}

	tests := map[string]copyto_test{
		"valid": {
			containerID: "test-container-id",
			src:         "foo",
		},
		"error": {
			containerID: "test-container-id",
		},
	}

	for desc, tst := range tests {
		var tempDir, fileName string
		var err error
		var file *os.File
		if len(tst.src) > 0 {
			tempDir, err = ioutil.TempDir("", tst.src)
			defer os.RemoveAll(tempDir)
			fileName = filepath.Join(tempDir, "bar")
			if err = os.MkdirAll(filepath.Dir(fileName), 0700); err == nil {
				file, err = os.Create(fileName)
				if err == nil {
					defer file.Close()
					file.WriteString("asdf")
				}
			}
		}
		if err != nil {
			t.Fatalf("Error creating src test files: %v", err)
		}
		fakeDocker := &test.FakeDockerClient{
			CopyToContainerContent: file,
		}
		dh := getDocker(fakeDocker)

		err = dh.UploadToContainer(fileName, fileName, tst.containerID)
		// the error we are inducing will prevent call into engine-api
		if len(tst.src) > 0 {
			if err != nil {
				t.Errorf("test case %s: Unexpected error returned: %v", desc, err)
			}
			if tst.containerID != fakeDocker.CopyToContainerID {
				t.Errorf("test case %s: copy to container called with unexpected id: %s and %s", desc, tst.containerID, fakeDocker.CopyToContainerID)
			}
		} else {
			if err == nil {
				t.Errorf("test case %s: Unexpected error returned: %v", desc, err)
			}
			if len(fakeDocker.CopyToContainerID) > 0 {
				t.Errorf("test case %s: copy to container called with unexpected id: %s and %s", desc, tst.containerID, fakeDocker.CopyToContainerID)
			}
		}
		// the directory of our file gets passed down to the engine-api method
		if tempDir != fakeDocker.CopyToContainerPath {
			t.Errorf("test case %s: copy to container called with unexpected path: %s and %s", desc, tempDir, fakeDocker.CopyToContainerPath)
		}
		// reflect.DeepEqual does not help here cause the reader is transformed prior to calling the engine-api stack, so just make sure it is no nil
		if file != nil && fakeDocker.CopyToContainerContent == nil {
			t.Errorf("test case %s: copy to container content was not passed through", desc)
		}
	}
}

func TestCopyFromContainer(t *testing.T) {
	type copyfrom_test struct {
		containerID   string
		srcPath       string
		expectedError error
	}

	tests := map[string]copyfrom_test{
		"valid": {
			containerID: "test-container-id",
			srcPath:     "/foo/bar",
		},
		"error": {
			containerID:   "test-container-id",
			srcPath:       "/foo/bar",
			expectedError: fmt.Errorf("Test error"),
		},
	}

	for desc, tst := range tests {
		buffer := bytes.NewBuffer([]byte(""))
		fakeDocker := &test.FakeDockerClient{
			CopyFromContainerErr: tst.expectedError,
		}
		dh := getDocker(fakeDocker)

		err := dh.DownloadFromContainer(tst.srcPath, buffer, tst.containerID)
		if err != tst.expectedError {
			t.Errorf("test case %s: Unexpected error returned: %v", desc, err)
		}
		if fakeDocker.CopyFromContainerID != tst.containerID {
			t.Errorf("test case %s: Unexpected container id: %s and %s", desc, tst.containerID, fakeDocker.CopyFromContainerID)
		}
		if fakeDocker.CopyFromContainerPath != tst.srcPath {
			t.Errorf("test case %s: Unexpected container id: %s and %s", desc, tst.srcPath, fakeDocker.CopyFromContainerPath)
		}
	}
}

func TestImageBuild(t *testing.T) {
	type wait_test struct {
		imageID       string
		expectedError error
	}

	tests := map[string]wait_test{
		"valid": {
			imageID: "test-container-id",
		},
		"error": {
			imageID:       "test-container-id",
			expectedError: fmt.Errorf("Test error"),
		},
	}

	for desc, tst := range tests {
		fakeDocker := &test.FakeDockerClient{
			BuildImageErr: tst.expectedError,
		}
		dh := getDocker(fakeDocker)
		opts := BuildImageOptions{
			Name: tst.imageID,
		}

		err := dh.BuildImage(opts)
		if err != tst.expectedError {
			t.Errorf("test case %s: Unexpected error returned: %v", desc, err)
		}
		if len(fakeDocker.BuildImageOpts.Tags) != 1 || fakeDocker.BuildImageOpts.Tags[0] != tst.imageID {
			t.Errorf("test case %s: Unexpected container id: %s and %+v", desc, tst.imageID, fakeDocker.BuildImageOpts.Tags)
		}
	}
}

func TestGetScriptsURL(t *testing.T) {
	type urltest struct {
		image       dockertypes.ImageInspect
		result      string
		calls       []string
		inspectErr  error
		errExpected bool
	}
	tests := map[string]urltest{
		"not present": {
			calls: []string{"inspect_image"},
			image: dockertypes.ImageInspect{
				ContainerConfig: &dockercontainer.Config{
					Env:    []string{"Env1=value1"},
					Labels: map[string]string{},
				},
				Config: &dockercontainer.Config{
					Env:    []string{"Env2=value2"},
					Labels: map[string]string{},
				},
			},
			result: "",
		},

		"env in containerConfig": {
			calls: []string{"inspect_image"},
			image: dockertypes.ImageInspect{
				ContainerConfig: &dockercontainer.Config{
					Env: []string{"Env1=value1", ScriptsURLEnvironment + "=test_url_value"},
				},
				Config: &dockercontainer.Config{},
			},
			result: "test_url_value",
		},

		"env in image config": {
			calls: []string{"inspect_image"},
			image: dockertypes.ImageInspect{
				ContainerConfig: &dockercontainer.Config{},
				Config: &dockercontainer.Config{
					Env: []string{
						"Env1=value1",
						ScriptsURLEnvironment + "=test_url_value_2",
						"Env2=value2",
					},
				},
			},
			result: "test_url_value_2",
		},

		"label in containerConfig": {
			calls: []string{"inspect_image"},
			image: dockertypes.ImageInspect{
				ContainerConfig: &dockercontainer.Config{
					Labels: map[string]string{ScriptsURLLabel: "test_url_value"},
				},
				Config: &dockercontainer.Config{},
			},
			result: "test_url_value",
		},

		"label in image config": {
			calls: []string{"inspect_image"},
			image: dockertypes.ImageInspect{
				ContainerConfig: &dockercontainer.Config{},
				Config: &dockercontainer.Config{
					Labels: map[string]string{ScriptsURLLabel: "test_url_value_2"},
				},
			},
			result: "test_url_value_2",
		},

		"inspect error": {
			calls:      []string{"inspect_image", "pull"},
			image:      dockertypes.ImageInspect{},
			inspectErr: fmt.Errorf("Inspect error"),
		},
	}
	for desc, tst := range tests {
		fakeDocker := &test.FakeDockerClient{}
		dh := getDocker(fakeDocker)
		fake := dh.kubeDockerClient.(*dockertools.FakeDockerClient)
		if tst.inspectErr != nil {
			fake.ClearErrors()
			fake.InjectError("pull", tst.inspectErr)
			fake.Image = nil
		} else {
			fake.Image = &tst.image
		}
		url, err := dh.GetScriptsURL("test/image")

		if e := fake.AssertCalls(tst.calls); e != nil {
			t.Errorf("%s: %+v", desc, e)
		}
		if err != nil && tst.inspectErr == nil {
			t.Errorf("%s: Unexpected error returned: %v", desc, err)
		} else if err == nil && tst.errExpected {
			t.Errorf("%s: Expected error. Did not get one.", desc)
		}
		if tst.inspectErr == nil && url != tst.result {
			t.Errorf("%s: Unexpected result. Expected: %s Actual: %s",
				desc, tst.result, url)
		}
	}
}

func TestRunContainer(t *testing.T) {
	type runtest struct {
		calls            []string
		image            dockertypes.ImageInspect
		cmd              string
		externalScripts  bool
		paramScriptsURL  string
		paramDestination string
		cmdExpected      []string
	}

	tests := map[string]runtest{
		"default": {
			calls: []string{"inspect_image", "inspect_image", "create", "start", "remove", "attach"},
			image: dockertypes.ImageInspect{
				ContainerConfig: &dockercontainer.Config{},
				Config:          &dockercontainer.Config{},
			},
			cmd:             api.Assemble,
			externalScripts: true,
			cmdExpected:     []string{"/bin/sh", "-c", fmt.Sprintf("tar -C /tmp -xf - && /tmp/scripts/%s", api.Assemble)},
		},
		"paramDestination": {
			calls: []string{"inspect_image", "inspect_image", "create", "start", "remove", "attach"},
			image: dockertypes.ImageInspect{
				ContainerConfig: &dockercontainer.Config{},
				Config:          &dockercontainer.Config{},
			},
			cmd:              api.Assemble,
			externalScripts:  true,
			paramDestination: "/opt/test",
			cmdExpected:      []string{"/bin/sh", "-c", fmt.Sprintf("tar -C /opt/test -xf - && /opt/test/scripts/%s", api.Assemble)},
		},
		"paramDestination&paramScripts": {
			calls: []string{"inspect_image", "inspect_image", "create", "start", "remove", "attach"},
			image: dockertypes.ImageInspect{
				ContainerConfig: &dockercontainer.Config{},
				Config:          &dockercontainer.Config{},
			},
			cmd:              api.Assemble,
			externalScripts:  true,
			paramDestination: "/opt/test",
			paramScriptsURL:  "http://my.test.url/test?param=one",
			cmdExpected:      []string{"/bin/sh", "-c", fmt.Sprintf("tar -C /opt/test -xf - && /opt/test/scripts/%s", api.Assemble)},
		},
		"scriptsInsideImageEnvironment": {
			calls: []string{"inspect_image", "inspect_image", "create", "start", "remove", "attach"},
			image: dockertypes.ImageInspect{
				ContainerConfig: &dockercontainer.Config{
					Env: []string{ScriptsURLEnvironment + "=image:///opt/bin/"},
				},
				Config: &dockercontainer.Config{},
			},
			cmd:             api.Assemble,
			externalScripts: false,
			cmdExpected:     []string{"/bin/sh", "-c", fmt.Sprintf("tar -C /tmp -xf - && /opt/bin/%s", api.Assemble)},
		},
		"scriptsInsideImageLabel": {
			calls: []string{"inspect_image", "inspect_image", "create", "start", "remove", "attach"},
			image: dockertypes.ImageInspect{
				ContainerConfig: &dockercontainer.Config{
					Labels: map[string]string{ScriptsURLLabel: "image:///opt/bin/"},
				},
				Config: &dockercontainer.Config{},
			},
			cmd:             api.Assemble,
			externalScripts: false,
			cmdExpected:     []string{"/bin/sh", "-c", fmt.Sprintf("tar -C /tmp -xf - && /opt/bin/%s", api.Assemble)},
		},
		"scriptsInsideImageEnvironmentWithParamDestination": {
			calls: []string{"inspect_image", "inspect_image", "create", "start", "remove", "attach"},
			image: dockertypes.ImageInspect{
				ContainerConfig: &dockercontainer.Config{
					Env: []string{ScriptsURLEnvironment + "=image:///opt/bin"},
				},
				Config: &dockercontainer.Config{},
			},
			cmd:              api.Assemble,
			externalScripts:  false,
			paramDestination: "/opt/sti",
			cmdExpected:      []string{"/bin/sh", "-c", fmt.Sprintf("tar -C /opt/sti -xf - && /opt/bin/%s", api.Assemble)},
		},
		"scriptsInsideImageLabelWithParamDestination": {
			calls: []string{"inspect_image", "inspect_image", "create", "start", "remove", "attach"},
			image: dockertypes.ImageInspect{
				ContainerConfig: &dockercontainer.Config{
					Labels: map[string]string{ScriptsURLLabel: "image:///opt/bin"},
				},
				Config: &dockercontainer.Config{},
			},
			cmd:              api.Assemble,
			externalScripts:  false,
			paramDestination: "/opt/sti",
			cmdExpected:      []string{"/bin/sh", "-c", fmt.Sprintf("tar -C /opt/sti -xf - && /opt/bin/%s", api.Assemble)},
		},
		"paramDestinationFromImageEnvironment": {
			calls: []string{"inspect_image", "inspect_image", "create", "start", "remove", "attach"},
			image: dockertypes.ImageInspect{
				ContainerConfig: &dockercontainer.Config{
					Env: []string{LocationEnvironment + "=/opt", ScriptsURLEnvironment + "=http://my.test.url/test?param=one"},
				},
				Config: &dockercontainer.Config{},
			},
			cmd:             api.Assemble,
			externalScripts: true,
			cmdExpected:     []string{"/bin/sh", "-c", fmt.Sprintf("tar -C /opt -xf - && /opt/scripts/%s", api.Assemble)},
		},
		"paramDestinationFromImageLabel": {
			calls: []string{"inspect_image", "inspect_image", "create", "start", "remove", "attach"},
			image: dockertypes.ImageInspect{
				ContainerConfig: &dockercontainer.Config{
					Labels: map[string]string{DestinationLabel: "/opt", ScriptsURLLabel: "http://my.test.url/test?param=one"},
				},
				Config: &dockercontainer.Config{},
			},
			cmd:             api.Assemble,
			externalScripts: true,
			cmdExpected:     []string{"/bin/sh", "-c", fmt.Sprintf("tar -C /opt -xf - && /opt/scripts/%s", api.Assemble)},
		},
		"usageCommand": {
			calls: []string{"inspect_image", "inspect_image", "create", "start", "remove", "attach"},
			image: dockertypes.ImageInspect{
				ContainerConfig: &dockercontainer.Config{},
				Config:          &dockercontainer.Config{},
			},
			cmd:             api.Usage,
			externalScripts: true,
			cmdExpected:     []string{"/bin/sh", "-c", fmt.Sprintf("tar -C /tmp -xf - && /tmp/scripts/%s", api.Usage)},
		},
		"otherCommand": {
			calls: []string{"inspect_image", "inspect_image", "create", "start", "remove", "attach"},
			image: dockertypes.ImageInspect{
				ContainerConfig: &dockercontainer.Config{},
				Config:          &dockercontainer.Config{},
			},
			cmd:             api.Run,
			externalScripts: true,
			cmdExpected:     []string{fmt.Sprintf("/tmp/scripts/%s", api.Run)},
		},
	}

	for desc, tst := range tests {
		fakeDocker := &test.FakeDockerClient{}
		dh := getDocker(fakeDocker)
		fake := dh.kubeDockerClient.(*dockertools.FakeDockerClient)
		tst.image.ID = "test/image"
		fake.Image = &tst.image
		if len(fake.ContainerMap) > 0 {
			t.Errorf("newly created fake client should have empty container map: %+v", fake.ContainerMap)
		}

		//NOTE: the combo of the fake k8s client, go 1.6, and using os.Stderr/os.Stdout caused what appeared to be go test crashes
		// when we tried to call their closers in RunContainer
		err := dh.RunContainer(RunContainerOptions{
			Image:           "test/image",
			PullImage:       true,
			ExternalScripts: tst.externalScripts,
			ScriptsURL:      tst.paramScriptsURL,
			Destination:     tst.paramDestination,
			Command:         tst.cmd,
			Env:             []string{"Key1=Value1", "Key2=Value2"},
			Stdin:           os.Stdin,
			//Stdout:          os.Stdout,
			//Stderr:          os.Stdout,
		})
		if err != nil {
			t.Errorf("%s: Unexpected error: %v", desc, err)
		}

		// container ID will be random, so don't look up directly ... just get the 1 entry which should be there
		if len(fake.ContainerMap) != 1 {
			t.Errorf("fake container map should only have 1 entry: %+v", fake.ContainerMap)
		}

		for _, container := range fake.ContainerMap {
			// Validate the Container parameters
			if container.Config == nil {
				t.Errorf("%s: container config not set: %+v", desc, container)
			}
			if container.Config.Image != "test/image:latest" {
				t.Errorf("%s: Unexpected create config image: %s", desc, container.Config.Image)
			}
			if !reflect.DeepEqual(container.Config.Cmd, dockerstrslice.StrSlice(tst.cmdExpected)) {
				t.Errorf("%s: Unexpected create config command: %#v instead of %q", desc, container.Config.Cmd, strings.Join(tst.cmdExpected, " "))
			}
			if !reflect.DeepEqual(container.Config.Env, []string{"Key1=Value1", "Key2=Value2"}) {
				t.Errorf("%s: Unexpected create config env: %#v", desc, container.Config.Env)
			}
		}

	}
}

func TestGetImageID(t *testing.T) {
	fakeDocker := &test.FakeDockerClient{}
	dh := getDocker(fakeDocker)
	fake := dh.kubeDockerClient.(*dockertools.FakeDockerClient)
	fake.Image = &dockertypes.ImageInspect{ID: "test-abcd"}
	id, err := dh.GetImageID("test/image")
	if e := fake.AssertCalls([]string{"inspect_image"}); e != nil {
		t.Errorf("%+v", e)
	}
	if err != nil {
		t.Errorf("Unexpected error returned: %v", err)
	} else if id != "test-abcd" {
		t.Errorf("Unexpected image id returned: %s", id)
	}
}

func TestGetImageIDError(t *testing.T) {
	expected := fmt.Errorf("Image Error")
	fakeDocker := &test.FakeDockerClient{}
	dh := getDocker(fakeDocker)
	fake := dh.kubeDockerClient.(*dockertools.FakeDockerClient)
	fake.Image = &dockertypes.ImageInspect{ID: "test-abcd"}
	fake.InjectError("inspect_image", expected)
	id, err := dh.GetImageID("test/image")
	if e := fake.AssertCalls([]string{"inspect_image"}); e != nil {
		t.Errorf("%+v", e)
	}
	if err != expected {
		t.Errorf("Unexpected error returned: %v", err)
	}
	if id != "" {
		t.Errorf("Unexpected image id returned: %s", id)
	}
}

func TestRemoveImage(t *testing.T) {
	fakeDocker := &test.FakeDockerClient{}
	dh := getDocker(fakeDocker)
	fake := dh.kubeDockerClient.(*dockertools.FakeDockerClient)
	fake.Images = []dockertypes.Image{{ID: "test-abcd"}}
	err := dh.RemoveImage("test-abcd")
	if err != nil {
		t.Errorf("Unexpected error removing image: %s", err)
	}
}

func TestGetImageName(t *testing.T) {
	type runtest struct {
		name     string
		expected string
	}
	tests := []runtest{
		{"test/image", "test/image:latest"},
		{"test/image:latest", "test/image:latest"},
		{"test/image:tag", "test/image:tag"},
		{"repository/test/image", "repository/test/image:latest"},
		{"repository/test/image:latest", "repository/test/image:latest"},
		{"repository/test/image:tag", "repository/test/image:tag"},
	}

	for _, tc := range tests {
		if e, a := tc.expected, getImageName(tc.name); e != a {
			t.Errorf("Expected image name %s, but got %s!", e, a)
		}
	}
}

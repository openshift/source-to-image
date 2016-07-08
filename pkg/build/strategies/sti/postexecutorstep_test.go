package sti

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/openshift/source-to-image/pkg/docker"
	"github.com/openshift/source-to-image/pkg/test"
)

func TestStorePreviousImageStep(t *testing.T) {
	testCases := []struct {
		imageIDError             error
		expectedPreviousImageID  string
		expectedPreviousImageTag string
	}{
		{
			imageIDError:             nil,
			expectedPreviousImageID:  "12345",
			expectedPreviousImageTag: "0.1",
		},
		{
			imageIDError:             fmt.Errorf("fail"),
			expectedPreviousImageID:  "",
			expectedPreviousImageTag: "0.1",
		},
	}

	for _, testCase := range testCases {

		builder := newFakeBaseSTI()
		builder.incremental = true
		builder.config.RemovePreviousImage = true
		builder.config.Tag = testCase.expectedPreviousImageTag

		docker := builder.docker.(*docker.FakeDocker)
		docker.GetImageIDResult = testCase.expectedPreviousImageID
		docker.GetImageIDError = testCase.imageIDError

		step := &storePreviousImageStep{builder: builder, docker: docker}

		ctx := &postExecutorStepContext{}

		if err := step.execute(ctx); err != nil {
			t.Fatalf("should exit without error, but it returned %v", err)
		}

		if docker.GetImageIDImage != testCase.expectedPreviousImageTag {
			t.Errorf("should invoke docker.GetImageID(%q) but invoked with %q", testCase.expectedPreviousImageTag, docker.GetImageIDImage)
		}

		if ctx.previousImageID != testCase.expectedPreviousImageID {
			t.Errorf("should set previousImageID field to %q but it's %q", testCase.expectedPreviousImageID, ctx.previousImageID)
		}
	}
}

func TestRemovePreviousImageStep(t *testing.T) {
	testCases := []struct {
		removeImageError        error
		expectedPreviousImageID string
	}{
		{
			removeImageError:        nil,
			expectedPreviousImageID: "",
		},
		{
			removeImageError:        nil,
			expectedPreviousImageID: "12345",
		},
		{
			removeImageError:        fmt.Errorf("fail"),
			expectedPreviousImageID: "12345",
		},
	}

	for _, testCase := range testCases {

		builder := newFakeBaseSTI()
		builder.incremental = true
		builder.config.RemovePreviousImage = true

		docker := builder.docker.(*docker.FakeDocker)
		docker.RemoveImageError = testCase.removeImageError

		step := &removePreviousImageStep{builder: builder, docker: docker}

		ctx := &postExecutorStepContext{previousImageID: testCase.expectedPreviousImageID}

		if err := step.execute(ctx); err != nil {
			t.Fatalf("should exit without error, but it returned %v", err)
		}

		if docker.RemoveImageName != testCase.expectedPreviousImageID {
			t.Errorf("should invoke docker.RemoveImage(%q) but invoked with %q", testCase.expectedPreviousImageID, docker.RemoveImageName)
		}
	}
}

func TestCommitImageStep(t *testing.T) {

	testCases := []struct {
		embeddedScript   bool
		destination      string
		expectedImageCmd string
	}{
		{
			embeddedScript:   false,
			destination:      "/path/to/location",
			expectedImageCmd: "/path/to/location/scripts/run",
		},
		{
			embeddedScript:   true,
			destination:      "image:///usr/bin/run.sh",
			expectedImageCmd: "/usr/bin/run.sh",
		},
	}

	for _, testCase := range testCases {

		expectedEnv := []string{"BUILD_LOGLEVEL"}
		expectedContainerID := "container-yyyy"
		expectedImageID := "image-xxx"
		expectedImageTag := "v1"
		expectedImageUser := "jboss"

		displayName := "MyApp"
		description := "My Application is awesome!"

		baseImageLabels := make(map[string]string)
		baseImageLabels["vendor"] = "CentOS"

		expectedLabels := make(map[string]string)
		expectedLabels["io.k8s.description"] = description
		expectedLabels["io.k8s.display-name"] = displayName
		expectedLabels["vendor"] = "CentOS"

		builder := newFakeBaseSTI()
		builder.config.DisplayName = displayName
		builder.config.Description = description
		builder.config.Tag = expectedImageTag
		builder.env = expectedEnv

		docker := builder.docker.(*docker.FakeDocker)
		docker.CommitContainerResult = expectedImageID
		docker.GetImageUserResult = expectedImageUser
		docker.Labels = baseImageLabels

		ctx := &postExecutorStepContext{
			containerID: expectedContainerID,
		}

		if testCase.embeddedScript {
			builder.scriptsURL = make(map[string]string)
			builder.scriptsURL["run"] = testCase.destination
		} else {
			ctx.destination = testCase.destination
		}

		step := &commitImageStep{builder: builder, docker: docker}

		if err := step.execute(ctx); err != nil {
			t.Fatalf("should exit without error, but it returned %v", err)
		}

		if ctx.imageID != expectedImageID {
			t.Errorf("should set ImageID field to %q but it's %q", expectedImageID, ctx.imageID)
		}

		commitOpts := docker.CommitContainerOpts

		if len(commitOpts.Command) != 1 {
			t.Errorf("should commit container with Command: %q, but commited with %q", testCase.expectedImageCmd, commitOpts.Command)

		} else if commitOpts.Command[0] != testCase.expectedImageCmd {
			t.Errorf("should commit container with Command: %q, but commited with %q", testCase.expectedImageCmd, commitOpts.Command[0])
		}

		if !reflect.DeepEqual(commitOpts.Env, expectedEnv) {
			t.Errorf("should commit container with Env: %v, but commited with %v", expectedEnv, commitOpts.Env)
		}

		if commitOpts.ContainerID != expectedContainerID {
			t.Errorf("should commit container with ContainerID: %q, but commited with %q", expectedContainerID, commitOpts.ContainerID)
		}

		if commitOpts.Repository != expectedImageTag {
			t.Errorf("should commit container with Repository: %q, but commited with %q", expectedImageTag, commitOpts.Repository)
		}

		if commitOpts.User != expectedImageUser {
			t.Errorf("should commit container with User: %q, but commited with %q", expectedImageUser, commitOpts.User)
		}

		if !reflect.DeepEqual(commitOpts.Labels, expectedLabels) {
			t.Errorf("should commit container with Labels: %v, but commited with %v", expectedLabels, commitOpts.Labels)
		}
	}
}

func TestDownloadFilesFromBuilderImageStep(t *testing.T) {
	// FIXME
}

func TestStartRuntimeImageAndUploadFilesStep(t *testing.T) {
	// FIXME
}

func TestReportSuccessStep(t *testing.T) {
	builder := newFakeBaseSTI()
	step := &reportSuccessStep{builder: builder}
	ctx := &postExecutorStepContext{imageID: "my-app"}

	if err := step.execute(ctx); err != nil {
		t.Fatalf("should exit without error, but it returned %v", err)
	}

	if builder.result.Success != true {
		t.Errorf("should set Success field to 'true' but it's %v", builder.result.Success)
	}

	if builder.result.ImageID != ctx.imageID {
		t.Errorf("should set ImageID field to %q but it's %q", ctx.imageID, builder.result.ImageID)
	}
}

func TestInvokeCallbackStep(t *testing.T) {
	expectedMessages := []string{"i'm", "ok"}
	expectedCallbackURL := "http://ping.me"
	builder := newFakeBaseSTI()
	builder.result.Success = true
	builder.result.Messages = expectedMessages
	builder.config.CallbackURL = expectedCallbackURL

	expectedResultMessages := []string{"all", "right"}
	callbackInvoker := &test.FakeCallbackInvoker{}
	callbackInvoker.Result = expectedResultMessages

	step := &invokeCallbackStep{
		builder:         builder,
		callbackInvoker: callbackInvoker,
	}

	expectedLabels := make(map[string]string)
	expectedLabels["result"] = "passed"
	ctx := &postExecutorStepContext{labels: expectedLabels}

	if err := step.execute(ctx); err != nil {
		t.Fatalf("should exit without error, but it returned %v", err)
	}

	if !reflect.DeepEqual(builder.result.Messages, expectedResultMessages) {
		t.Errorf("should set Messages field to %q but it's %q", expectedResultMessages, builder.result.Messages)
	}

	if callbackInvoker.CallbackURL != expectedCallbackURL {
		t.Errorf("should invoke ExecuteCallback(CallbackURL=%q) but invoked with %q", expectedCallbackURL, callbackInvoker.CallbackURL)
	}

	if callbackInvoker.Success != true {
		t.Errorf("should invoke ExecuteCallback(Success='true') but invoked with %v", callbackInvoker.Success)
	}

	if !reflect.DeepEqual(callbackInvoker.Messages, expectedMessages) {
		t.Errorf("should invoke ExecuteCallback(Messages=%v) but invoked with %v", expectedMessages, callbackInvoker.Messages)
	}

	if !reflect.DeepEqual(callbackInvoker.Labels, expectedLabels) {
		t.Errorf("should invoke ExecuteCallback(Labels=%v) but invoked with %v", expectedLabels, callbackInvoker.Labels)
	}
}

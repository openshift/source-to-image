package sti

import (
	"archive/tar"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/openshift/source-to-image/pkg/api"
	dockerpkg "github.com/openshift/source-to-image/pkg/docker"
	"github.com/openshift/source-to-image/pkg/errors"
	s2itar "github.com/openshift/source-to-image/pkg/tar"
	"github.com/openshift/source-to-image/pkg/util"
	utilstatus "github.com/openshift/source-to-image/pkg/util/status"
)

type postExecutorStepContext struct {
	// id of the previous image that we're holding because after committing the image, we'll lose it.
	// Used only when build is incremental and RemovePreviousImage setting is enabled.
	// See also: storePreviousImageStep and removePreviousImageStep
	previousImageID string

	// Container id that will be committed.
	// See also: commitImageStep
	containerID string

	// Path to a directory in the image where scripts (for example, "run") will be placed.
	// This location will be used for generation of the CMD directive.
	// See also: commitImageStep
	destination string

	// Image id created by committing the container.
	// See also: commitImageStep and reportAboutSuccessStep
	imageID string

	// Labels that will be passed to a callback.
	// These labels are added to the image during commit.
	// See also: commitImageStep and STI.Build()
	labels map[string]string
}

type postExecutorStep interface {
	execute(*postExecutorStepContext) error
}

type storePreviousImageStep struct {
	builder *STI
	docker  dockerpkg.Docker
}

func (step *storePreviousImageStep) execute(ctx *postExecutorStepContext) error {
	if step.builder.incremental && step.builder.config.RemovePreviousImage {
		glog.V(3).Info("Executing step: store previous image")
		ctx.previousImageID = step.getPreviousImage()
		return nil
	}

	glog.V(3).Info("Skipping step: store previous image")
	return nil
}

func (step *storePreviousImageStep) getPreviousImage() string {
	previousImageID, err := step.docker.GetImageID(step.builder.config.Tag)
	if err != nil {
		glog.V(0).Infof("error: Error retrieving previous image's (%v) metadata: %v", step.builder.config.Tag, err)
		return ""
	}
	return previousImageID
}

type removePreviousImageStep struct {
	builder *STI
	docker  dockerpkg.Docker
}

func (step *removePreviousImageStep) execute(ctx *postExecutorStepContext) error {
	if step.builder.incremental && step.builder.config.RemovePreviousImage {
		glog.V(3).Info("Executing step: remove previous image")
		step.removePreviousImage(ctx.previousImageID)
		return nil
	}

	glog.V(3).Info("Skipping step: remove previous image")
	return nil
}

func (step *removePreviousImageStep) removePreviousImage(previousImageID string) {
	if previousImageID == "" {
		return
	}

	glog.V(1).Infof("Removing previously-tagged image %s", previousImageID)
	if err := step.docker.RemoveImage(previousImageID); err != nil {
		glog.V(0).Infof("error: Unable to remove previous image: %v", err)
	}
}

type commitImageStep struct {
	image   string
	builder *STI
	docker  dockerpkg.Docker
}

func (step *commitImageStep) execute(ctx *postExecutorStepContext) error {
	glog.V(3).Infof("Executing step: commit image")

	user, err := step.docker.GetImageUser(step.image)
	if err != nil {
		return fmt.Errorf("Couldn't get user of %q image: %v", step.image, err)
	}

	cmd := createCommandForExecutingRunScript(step.builder.scriptsURL, ctx.destination)

	ctx.labels = createLabelsForResultingImage(step.builder, step.docker, step.image)

	// Set the image entrypoint back to its original value on commit, the running
	// container has "env" as its entrypoint and we don't want to commit that.
	entrypoint, err := step.docker.GetImageEntrypoint(step.image)
	if err != nil {
		return fmt.Errorf("Couldn't get entrypoint of %q image: %v", step.image, err)
	}
	// If the image has no explicit entrypoint, set it to an empty array
	// so we don't default to leaving the entrypoint as "env" upon commit.
	if entrypoint == nil {
		entrypoint = []string{}
	}

	ctx.imageID, err = commitContainer(
		step.docker,
		ctx.containerID,
		cmd,
		user,
		step.builder.config.Tag,
		step.builder.env,
		entrypoint,
		ctx.labels,
	)

	if err != nil {
		step.builder.result.BuildInfo.FailureReason = utilstatus.NewFailureReason(
			utilstatus.ReasonCommitContainerFailed,
			utilstatus.ReasonMessageCommitContainerFailed,
		)
		return err
	}

	return nil
}

type downloadFilesFromBuilderImageStep struct {
	builder *STI
	docker  dockerpkg.Docker
	fs      util.FileSystem
	tar     s2itar.Tar
}

func (step *downloadFilesFromBuilderImageStep) execute(ctx *postExecutorStepContext) error {
	glog.V(3).Info("Executing step: download files from the builder image")

	artifactsDir := filepath.Join(step.builder.config.WorkingDir, api.RuntimeArtifactsDir)
	if err := step.fs.Mkdir(artifactsDir); err != nil {
		step.builder.result.BuildInfo.FailureReason = utilstatus.NewFailureReason(
			utilstatus.ReasonFSOperationFailed,
			utilstatus.ReasonMessageFSOperationFailed,
		)
		return fmt.Errorf("Couldn't create directory %q: %v", artifactsDir, err)
	}

	for _, artifact := range step.builder.config.RuntimeArtifacts {
		if err := step.downloadAndExtractFile(artifact.Source, artifactsDir, ctx.containerID); err != nil {
			step.builder.result.BuildInfo.FailureReason = utilstatus.NewFailureReason(
				utilstatus.ReasonGenericS2IBuildFailed,
				utilstatus.ReasonMessageGenericS2iBuildFailed,
			)
			return err
		}

		// for mapping like "/tmp/foo.txt -> app" we should create "app" and move "foo.txt" to that directory
		dstSubDir := path.Clean(artifact.Destination)
		if dstSubDir != "." && dstSubDir != "/" {
			dstDir := filepath.Join(artifactsDir, dstSubDir)
			glog.V(5).Infof("Creating directory %q", dstDir)
			if err := step.fs.MkdirAll(dstDir); err != nil {
				step.builder.result.BuildInfo.FailureReason = utilstatus.NewFailureReason(
					utilstatus.ReasonFSOperationFailed,
					utilstatus.ReasonMessageFSOperationFailed,
				)
				return fmt.Errorf("Couldn't create directory %q: %v", dstDir, err)
			}

			file := filepath.Base(artifact.Source)
			old := filepath.Join(artifactsDir, file)
			new := filepath.Join(artifactsDir, dstSubDir, file)
			glog.V(5).Infof("Renaming %q to %q", old, new)
			if err := step.fs.Rename(old, new); err != nil {
				step.builder.result.BuildInfo.FailureReason = utilstatus.NewFailureReason(
					utilstatus.ReasonFSOperationFailed,
					utilstatus.ReasonMessageFSOperationFailed,
				)
				return fmt.Errorf("Couldn't rename %q -> %q: %v", old, new, err)
			}
		}
	}

	return nil
}

func (step *downloadFilesFromBuilderImageStep) downloadAndExtractFile(artifactPath, artifactsDir, containerID string) error {
	glog.V(5).Infof("Downloading file %q", artifactPath)

	fd, err := ioutil.TempFile(artifactsDir, "s2i-runtime-artifact")
	if err != nil {
		return fmt.Errorf("Couldn't create temporary file for runtime artifact: %v", err)
	}
	defer func() {
		fd.Close()
		os.Remove(fd.Name())
	}()

	if err := step.docker.DownloadFromContainer(artifactPath, fd, containerID); err != nil {
		return fmt.Errorf("Couldn't download file (%q -> %q) from container %s: %v", artifactPath, fd.Name(), containerID, err)
	}

	// after writing to the file descriptor we need to rewind pointer to the beginning of the file before next reading
	if _, err := fd.Seek(0, os.SEEK_SET); err != nil {
		return fmt.Errorf("Couldn't seek to the beginning of the file %q: %v", fd.Name(), err)
	}

	if err := step.tar.ExtractTarStream(artifactsDir, fd); err != nil {
		return fmt.Errorf("Couldn't extract runtime artifact %q into the directory %q: %v", artifactPath, artifactsDir, err)
	}

	return nil
}

type startRuntimeImageAndUploadFilesStep struct {
	builder *STI
	docker  dockerpkg.Docker
	fs      util.FileSystem
}

func (step *startRuntimeImageAndUploadFilesStep) execute(ctx *postExecutorStepContext) error {
	glog.V(3).Info("Executing step: start runtime image and upload files")

	fd, err := ioutil.TempFile("", "s2i-upload-done")
	if err != nil {
		return err
	}
	fd.Close()
	lastFilePath := fd.Name()
	defer func() {
		os.Remove(lastFilePath)
	}()

	lastFileDstPath := "/tmp/" + filepath.Base(lastFilePath)

	outReader, outWriter := io.Pipe()
	errReader, errWriter := io.Pipe()

	artifactsDir := filepath.Join(step.builder.config.WorkingDir, api.RuntimeArtifactsDir)

	// We copy scripts to a directory with artifacts to upload files in one shot
	for _, script := range []string{api.AssembleRuntime, api.Run} {
		// scripts must be inside of "scripts" subdir, see createCommandForExecutingRunScript()
		destinationDir := filepath.Join(artifactsDir, "scripts")
		err = step.copyScriptIfNeeded(script, destinationDir)
		if err != nil {
			return err
		}
	}

	image := step.builder.config.RuntimeImage
	workDir, err := step.docker.GetImageWorkdir(image)
	if err != nil {
		return fmt.Errorf("Couldn't get working dir of %q image: %v", image, err)
	}

	commandBaseDir := filepath.Join(workDir, "scripts")
	useExternalAssembleScript := step.builder.externalScripts[api.AssembleRuntime]
	if !useExternalAssembleScript {
		// script already inside of the image
		var scriptsURL string
		scriptsURL, err = step.docker.GetScriptsURL(image)
		if err != nil {
			return err
		}
		if len(scriptsURL) == 0 {
			return fmt.Errorf("Couldn't determine scripts URL for image %q", image)
		}
		commandBaseDir = strings.TrimPrefix(scriptsURL, "image://")
	}

	cmd := fmt.Sprintf(
		"while [ ! -f %q ]; do sleep 0.5; done; %s/%s; exit $?",
		lastFileDstPath,
		commandBaseDir,
		api.AssembleRuntime,
	)

	opts := dockerpkg.RunContainerOptions{
		Image:           image,
		PullImage:       false, // The PullImage is false because we've already pulled the image
		CommandExplicit: []string{"/bin/sh", "-c", cmd},
		Stdout:          outWriter,
		Stderr:          errWriter,
		NetworkMode:     string(step.builder.config.DockerNetworkMode),
		CGroupLimits:    step.builder.config.CGroupLimits,
		CapDrop:         step.builder.config.DropCapabilities,
		PostExec:        step.builder.postExecutor,
		Env:             step.builder.env,
	}

	opts.OnStart = func(containerID string) error {
		setStandardPerms := func(writer io.Writer) s2itar.Writer {
			return s2itar.ChmodAdapter{Writer: tar.NewWriter(writer), NewFileMode: 0644, NewExecFileMode: 0755, NewDirMode: 0755}
		}

		glog.V(5).Infof("Uploading directory %q -> %q", artifactsDir, workDir)
		onStartErr := step.docker.UploadToContainerWithTarWriter(step.fs, artifactsDir, workDir, containerID, setStandardPerms)
		if onStartErr != nil {
			return fmt.Errorf("Couldn't upload directory (%q -> %q) into container %s: %v", artifactsDir, workDir, containerID, err)
		}

		glog.V(5).Infof("Uploading file %q -> %q", lastFilePath, lastFileDstPath)
		onStartErr = step.docker.UploadToContainerWithTarWriter(step.fs, lastFilePath, lastFileDstPath, containerID, setStandardPerms)
		if onStartErr != nil {
			return fmt.Errorf("Couldn't upload file (%q -> %q) into container %s: %v", lastFilePath, lastFileDstPath, containerID, err)
		}

		return onStartErr
	}

	dockerpkg.StreamContainerIO(outReader, nil, func(s string) { glog.V(0).Info(s) })

	errOutput := ""
	c := dockerpkg.StreamContainerIO(errReader, &errOutput, func(s string) { glog.Info(s) })

	// switch to the next stage of post executors steps
	step.builder.postExecutorStage++

	err = step.docker.RunContainer(opts)
	if e, ok := err.(errors.ContainerError); ok {
		// Must wait for StreamContainerIO goroutine above to exit before reading errOutput.
		<-c
		err = errors.NewContainerError(image, e.ErrorCode, errOutput)
	}

	return err
}

func (step *startRuntimeImageAndUploadFilesStep) copyScriptIfNeeded(script, destinationDir string) error {
	useExternalScript := step.builder.externalScripts[script]
	if useExternalScript {
		src := filepath.Join(step.builder.config.WorkingDir, api.UploadScripts, script)
		dst := filepath.Join(destinationDir, script)
		glog.V(5).Infof("Copying file %q -> %q", src, dst)
		if err := step.fs.MkdirAll(destinationDir); err != nil {
			return fmt.Errorf("Couldn't create directory %q: %v", destinationDir, err)
		}
		if err := step.fs.Copy(src, dst); err != nil {
			return fmt.Errorf("Couldn't copy file (%q -> %q): %v", src, dst, err)
		}
	}
	return nil
}

type reportSuccessStep struct {
	builder *STI
}

func (step *reportSuccessStep) execute(ctx *postExecutorStepContext) error {
	glog.V(3).Info("Executing step: report success")

	step.builder.result.Success = true
	step.builder.result.ImageID = ctx.imageID

	glog.V(3).Infof("Successfully built %s", firstNonEmpty(step.builder.config.Tag, ctx.imageID))

	return nil
}

// shared methods

func commitContainer(docker dockerpkg.Docker, containerID, cmd, user, tag string, env, entrypoint []string, labels map[string]string) (string, error) {
	opts := dockerpkg.CommitContainerOptions{
		Command:     []string{cmd},
		Env:         env,
		Entrypoint:  entrypoint,
		ContainerID: containerID,
		Repository:  tag,
		User:        user,
		Labels:      labels,
	}

	imageID, err := docker.CommitContainer(opts)
	if err != nil {
		return "", errors.NewCommitError(tag, err)
	}

	return imageID, nil
}

func createLabelsForResultingImage(builder *STI, docker dockerpkg.Docker, baseImage string) map[string]string {
	generatedLabels := util.GenerateOutputImageLabels(builder.sourceInfo, builder.config)

	existingLabels, err := docker.GetLabels(baseImage)
	if err != nil {
		glog.V(0).Infof("error: Unable to read existing labels from the base image %s", baseImage)
	}

	configLabels := builder.config.Labels

	return mergeLabels(configLabels, generatedLabels, existingLabels)
}

func mergeLabels(configLabels, generatedLabels, existingLabels map[string]string) map[string]string {
	result := map[string]string{}
	for k, v := range existingLabels {
		result[k] = v
	}
	for k, v := range generatedLabels {
		result[k] = v
	}
	for k, v := range configLabels {
		result[k] = v
	}
	return result
}

func createCommandForExecutingRunScript(scriptsURL map[string]string, location string) string {
	cmd := scriptsURL[api.Run]
	if strings.HasPrefix(cmd, "image://") {
		// scripts from inside of the image, we need to strip the image part
		// NOTE: We use path.Join instead of filepath.Join to avoid converting the
		// path to UNC (Windows) format as we always run this inside container.
		cmd = strings.TrimPrefix(cmd, "image://")
	} else {
		// external scripts, in which case we're taking the directory to which they
		// were extracted and append scripts dir and name
		cmd = path.Join(location, "scripts", api.Run)
	}
	return cmd
}

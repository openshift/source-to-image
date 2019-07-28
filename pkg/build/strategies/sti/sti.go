package sti

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/api/constants"
	"github.com/openshift/source-to-image/pkg/build"
	"github.com/openshift/source-to-image/pkg/build/strategies/layered"
	dockerpkg "github.com/openshift/source-to-image/pkg/docker"
	s2ierr "github.com/openshift/source-to-image/pkg/errors"
	"github.com/openshift/source-to-image/pkg/ignore"
	"github.com/openshift/source-to-image/pkg/scm"
	"github.com/openshift/source-to-image/pkg/scm/git"
	"github.com/openshift/source-to-image/pkg/scripts"
	"github.com/openshift/source-to-image/pkg/tar"
	"github.com/openshift/source-to-image/pkg/util"
	"github.com/openshift/source-to-image/pkg/util/cmd"
	"github.com/openshift/source-to-image/pkg/util/fs"
	utillog "github.com/openshift/source-to-image/pkg/util/log"
	utilstatus "github.com/openshift/source-to-image/pkg/util/status"
)

const (
	injectionResultFile = "/tmp/injection-result"
	rmInjectionsScript  = "/tmp/rm-injections"
)

var (
	log = utillog.StderrLog

	// List of directories that needs to be present inside working dir
	workingDirs = []string{
		constants.UploadScripts,
		constants.Source,
		constants.DefaultScripts,
		constants.UserScripts,
	}

	errMissingRequirements = errors.New("missing requirements")
)

// STI strategy executes the S2I build.
// For more details about S2I, visit https://github.com/openshift/source-to-image
type STI struct {
	config                 *api.Config
	result                 *api.Result
	postExecutor           dockerpkg.PostExecutor
	installer              scripts.Installer
	runtimeInstaller       scripts.Installer
	git                    git.Git
	fs                     fs.FileSystem
	tar                    tar.Tar
	docker                 dockerpkg.Docker
	incrementalDocker      dockerpkg.Docker
	runtimeDocker          dockerpkg.Docker
	callbackInvoker        util.CallbackInvoker
	requiredScripts        []string
	optionalScripts        []string
	optionalRuntimeScripts []string
	externalScripts        map[string]bool
	installedScripts       map[string]bool
	scriptsURL             map[string]string
	incremental            bool
	sourceInfo             *git.SourceInfo
	env                    []string
	newLabels              map[string]string

	// Interfaces
	preparer  build.Preparer
	ignorer   build.Ignorer
	artifacts build.IncrementalBuilder
	scripts   build.ScriptsHandler
	source    build.Downloader
	garbage   build.Cleaner
	layered   build.Builder

	// post executors steps
	postExecutorStage            int
	postExecutorFirstStageSteps  []postExecutorStep
	postExecutorSecondStageSteps []postExecutorStep
	postExecutorStepsContext     *postExecutorStepContext
}

// New returns the instance of STI builder strategy for the given config.
// If the layeredBuilder parameter is specified, then the builder provided will
// be used for the case that the base Docker image does not have 'tar' or 'bash'
// installed.
func New(client dockerpkg.Client, config *api.Config, fs fs.FileSystem, overrides build.Overrides) (*STI, error) {
	excludePattern, err := regexp.Compile(config.ExcludeRegExp)
	if err != nil {
		return nil, err
	}

	docker := dockerpkg.New(client, config.PullAuthentication)
	var incrementalDocker dockerpkg.Docker
	if config.Incremental {
		incrementalDocker = dockerpkg.New(client, config.IncrementalAuthentication)
	}

	inst := scripts.NewInstaller(
		config.BuilderImage,
		config.ScriptsURL,
		config.ScriptDownloadProxyConfig,
		docker,
		config.PullAuthentication,
		fs,
	)
	tarHandler := tar.NewParanoid(fs)
	tarHandler.SetExclusionPattern(excludePattern)

	builder := &STI{
		installer:              inst,
		config:                 config,
		docker:                 docker,
		incrementalDocker:      incrementalDocker,
		git:                    git.New(fs, cmd.NewCommandRunner()),
		fs:                     fs,
		tar:                    tarHandler,
		callbackInvoker:        util.NewCallbackInvoker(),
		requiredScripts:        scripts.RequiredScripts,
		optionalScripts:        scripts.OptionalScripts,
		optionalRuntimeScripts: []string{constants.AssembleRuntime},
		externalScripts:        map[string]bool{},
		installedScripts:       map[string]bool{},
		scriptsURL:             map[string]string{},
		newLabels:              map[string]string{},
	}

	if len(config.RuntimeImage) > 0 {
		builder.runtimeDocker = dockerpkg.New(client, config.RuntimeAuthentication)

		builder.runtimeInstaller = scripts.NewInstaller(
			config.RuntimeImage,
			config.ScriptsURL,
			config.ScriptDownloadProxyConfig,
			builder.runtimeDocker,
			config.RuntimeAuthentication,
			builder.fs,
		)
	}

	// The sources are downloaded using the Git downloader.
	// TODO: Add more SCM in future.
	// TODO: explicit decision made to customize processing for usage specifically vs.
	// leveraging overrides; also, we ultimately want to simplify s2i usage a good bit,
	// which would lead to replacing this quick short circuit (so this change is tactical)
	builder.source = overrides.Downloader
	if builder.source == nil && !config.Usage {
		downloader, err := scm.DownloaderForSource(builder.fs, config.Source, config.ForceCopy)
		if err != nil {
			return nil, err
		}
		builder.source = downloader
	}
	builder.garbage = build.NewDefaultCleaner(builder.fs, builder.docker)

	builder.layered, err = layered.New(client, config, builder.fs, builder, overrides)
	if err != nil {
		return nil, err
	}

	// Set interfaces
	builder.preparer = builder
	// later on, if we support say .gitignore func in addition to .dockerignore
	// func, setting ignorer will be based on config setting
	builder.ignorer = &ignore.DockerIgnorer{}
	builder.artifacts = builder
	builder.scripts = builder
	builder.postExecutor = builder
	builder.initPostExecutorSteps()

	return builder, nil
}

// Build processes a Request and returns a *api.Result and an error.
// An error represents a failure performing the build rather than a failure
// of the build itself.  Callers should check the Success field of the result
// to determine whether a build succeeded or not.
func (builder *STI) Build(config *api.Config) (*api.Result, error) {
	builder.result = &api.Result{}

	if len(builder.config.CallbackURL) > 0 {
		defer func() {
			builder.result.Messages = builder.callbackInvoker.ExecuteCallback(
				builder.config.CallbackURL,
				builder.result.Success,
				builder.postExecutorStepsContext.labels,
				builder.result.Messages,
			)
		}()
	}
	defer builder.garbage.Cleanup(config)

	log.V(1).Infof("Preparing to build %s", config.Tag)
	if err := builder.preparer.Prepare(config); err != nil {
		return builder.result, err
	}

	if builder.incremental = builder.artifacts.Exists(config); builder.incremental {
		tag := util.FirstNonEmpty(config.IncrementalFromTag, config.Tag)
		log.V(1).Infof("Existing image for tag %s detected for incremental build", tag)
	} else {
		log.V(1).Info("Clean build will be performed")
	}

	log.V(2).Infof("Performing source build from %s", config.Source)
	if builder.incremental {
		if err := builder.artifacts.Save(config); err != nil {
			log.Warning("Clean build will be performed because of error saving previous build artifacts")
			log.V(2).Infof("error: %v", err)
		}
	}

	if len(config.AssembleUser) > 0 {
		log.V(1).Infof("Running %q in %q as %q user", constants.Assemble, config.Tag, config.AssembleUser)
	} else {
		log.V(1).Infof("Running %q in %q", constants.Assemble, config.Tag)
	}
	startTime := time.Now()
	if err := builder.scripts.Execute(constants.Assemble, config.AssembleUser, config); err != nil {
		if err == errMissingRequirements {
			log.V(1).Info("Image is missing basic requirements (sh or tar), layered build will be performed")
			return builder.layered.Build(config)
		}
		if e, ok := err.(s2ierr.ContainerError); ok {
			if !isMissingRequirements(e.Output) {
				builder.result.BuildInfo.FailureReason = utilstatus.NewFailureReason(
					utilstatus.ReasonAssembleFailed,
					utilstatus.ReasonMessageAssembleFailed,
				)
				return builder.result, err
			}
			log.V(1).Info("Image is missing basic requirements (sh or tar), layered build will be performed")
			buildResult, err := builder.layered.Build(config)
			return buildResult, err
		}

		return builder.result, err
	}
	builder.result.BuildInfo.Stages = api.RecordStageAndStepInfo(builder.result.BuildInfo.Stages, api.StageAssemble, api.StepAssembleBuildScripts, startTime, time.Now())
	builder.result.Success = true

	return builder.result, nil
}

// Prepare prepares the source code and tar for build.
// NOTE: this func serves both the sti and onbuild strategies, as the OnBuild
// struct Build func leverages the STI struct Prepare func directly below.
func (builder *STI) Prepare(config *api.Config) error {
	var err error
	if builder.result == nil {
		builder.result = &api.Result{}
	}

	if len(config.WorkingDir) == 0 {
		if config.WorkingDir, err = builder.fs.CreateWorkingDirectory(); err != nil {
			builder.result.BuildInfo.FailureReason = utilstatus.NewFailureReason(
				utilstatus.ReasonFSOperationFailed,
				utilstatus.ReasonMessageFSOperationFailed,
			)
			return err
		}
	}

	builder.result.WorkingDir = config.WorkingDir

	if len(config.RuntimeImage) > 0 {
		startTime := time.Now()
		dockerpkg.GetRuntimeImage(builder.runtimeDocker, config)
		builder.result.BuildInfo.Stages = api.RecordStageAndStepInfo(builder.result.BuildInfo.Stages, api.StagePullImages, api.StepPullRuntimeImage, startTime, time.Now())

		if err != nil {
			builder.result.BuildInfo.FailureReason = utilstatus.NewFailureReason(
				utilstatus.ReasonPullRuntimeImageFailed,
				utilstatus.ReasonMessagePullRuntimeImageFailed,
			)
			log.Errorf("Unable to pull runtime image %q: %v", config.RuntimeImage, err)
			return err
		}

		// user didn't specify mapping, let's take it from the runtime image then
		if len(builder.config.RuntimeArtifacts) == 0 {
			var mapping string
			mapping, err = builder.docker.GetAssembleInputFiles(config.RuntimeImage)
			if err != nil {
				builder.result.BuildInfo.FailureReason = utilstatus.NewFailureReason(
					utilstatus.ReasonInvalidArtifactsMapping,
					utilstatus.ReasonMessageInvalidArtifactsMapping,
				)
				return err
			}
			if len(mapping) == 0 {
				builder.result.BuildInfo.FailureReason = utilstatus.NewFailureReason(
					utilstatus.ReasonGenericS2IBuildFailed,
					utilstatus.ReasonMessageGenericS2iBuildFailed,
				)
				return errors.New("no runtime artifacts to copy were specified")
			}
			for _, value := range strings.Split(mapping, ";") {
				if err = builder.config.RuntimeArtifacts.Set(value); err != nil {
					builder.result.BuildInfo.FailureReason = utilstatus.NewFailureReason(
						utilstatus.ReasonGenericS2IBuildFailed,
						utilstatus.ReasonMessageGenericS2iBuildFailed,
					)
					return fmt.Errorf("could not  parse %q label with value %q on image %q: %v",
						constants.AssembleInputFilesLabel, mapping, config.RuntimeImage, err)
				}
			}
		}

		if len(config.AssembleRuntimeUser) == 0 {
			if config.AssembleRuntimeUser, err = builder.docker.GetAssembleRuntimeUser(config.RuntimeImage); err != nil {
				builder.result.BuildInfo.FailureReason = utilstatus.NewFailureReason(
					utilstatus.ReasonGenericS2IBuildFailed,
					utilstatus.ReasonMessageGenericS2iBuildFailed,
				)
				return fmt.Errorf("could not get %q label value on image %q: %v",
					constants.AssembleRuntimeUserLabel, config.RuntimeImage, err)
			}
		}

		// we're validating values here to be sure that we're handling both of the cases of the invocation:
		// from main() and as a method from OpenShift
		for _, volumeSpec := range builder.config.RuntimeArtifacts {
			var volumeErr error

			switch {
			case !path.IsAbs(filepath.ToSlash(volumeSpec.Source)):
				volumeErr = fmt.Errorf("invalid runtime artifacts mapping: %q -> %q: source must be an absolute path", volumeSpec.Source, volumeSpec.Destination)
			case path.IsAbs(volumeSpec.Destination):
				volumeErr = fmt.Errorf("invalid runtime artifacts mapping: %q -> %q: destination must be a relative path", volumeSpec.Source, volumeSpec.Destination)
			case strings.HasPrefix(volumeSpec.Destination, ".."):
				volumeErr = fmt.Errorf("invalid runtime artifacts mapping: %q -> %q: destination cannot start with '..'", volumeSpec.Source, volumeSpec.Destination)
			default:
				continue
			}
			if volumeErr != nil {
				builder.result.BuildInfo.FailureReason = utilstatus.NewFailureReason(
					utilstatus.ReasonInvalidArtifactsMapping,
					utilstatus.ReasonMessageInvalidArtifactsMapping,
				)
				return volumeErr
			}
		}
	}

	// Setup working directories
	for _, v := range workingDirs {
		if err = builder.fs.MkdirAllWithPermissions(filepath.Join(config.WorkingDir, v), 0755); err != nil {
			builder.result.BuildInfo.FailureReason = utilstatus.NewFailureReason(
				utilstatus.ReasonFSOperationFailed,
				utilstatus.ReasonMessageFSOperationFailed,
			)
			return err
		}
	}

	// fetch sources, for their .s2i/bin might contain s2i scripts
	if config.Source != nil {
		if builder.sourceInfo, err = builder.source.Download(config); err != nil {
			builder.result.BuildInfo.FailureReason = utilstatus.NewFailureReason(
				utilstatus.ReasonFetchSourceFailed,
				utilstatus.ReasonMessageFetchSourceFailed,
			)
			return err
		}
		if config.SourceInfo != nil {
			builder.sourceInfo = config.SourceInfo
		}
	}

	// get the scripts
	required, err := builder.installer.InstallRequired(builder.requiredScripts, config.WorkingDir)
	if err != nil {
		builder.result.BuildInfo.FailureReason = utilstatus.NewFailureReason(
			utilstatus.ReasonInstallScriptsFailed,
			utilstatus.ReasonMessageInstallScriptsFailed,
		)
		return err
	}
	optional := builder.installer.InstallOptional(builder.optionalScripts, config.WorkingDir)

	requiredAndOptional := append(required, optional...)

	if len(config.RuntimeImage) > 0 && builder.runtimeInstaller != nil {
		optionalRuntime := builder.runtimeInstaller.InstallOptional(builder.optionalRuntimeScripts, config.WorkingDir)
		requiredAndOptional = append(requiredAndOptional, optionalRuntime...)
	}

	// If a ScriptsURL was specified, but no scripts were downloaded from it, throw an error
	if len(config.ScriptsURL) > 0 {
		failedCount := 0
		for _, result := range requiredAndOptional {
			if util.Includes(result.FailedSources, scripts.ScriptURLHandler) {
				failedCount++
			}
		}
		if failedCount == len(requiredAndOptional) {
			builder.result.BuildInfo.FailureReason = utilstatus.NewFailureReason(
				utilstatus.ReasonScriptsFetchFailed,
				utilstatus.ReasonMessageScriptsFetchFailed,
			)
			return fmt.Errorf("could not download any scripts from URL %v", config.ScriptsURL)
		}
	}

	for _, r := range requiredAndOptional {
		if r.Error != nil {
			log.Warningf("Error getting %v from %s: %v", r.Script, r.URL, r.Error)
			continue
		}

		builder.externalScripts[r.Script] = r.Downloaded
		builder.installedScripts[r.Script] = r.Installed
		builder.scriptsURL[r.Script] = r.URL
	}

	// see if there is a .s2iignore file, and if so, read in the patterns an then
	// search and delete on
	return builder.ignorer.Ignore(config)
}

// SetScripts allows to override default required and optional scripts
func (builder *STI) SetScripts(required, optional []string) {
	builder.requiredScripts = required
	builder.optionalScripts = optional
}

// PostExecute allows to execute post-build actions after the Docker
// container execution finishes.
func (builder *STI) PostExecute(containerID, destination string) error {
	builder.postExecutorStepsContext.containerID = containerID
	builder.postExecutorStepsContext.destination = destination

	stageSteps := builder.postExecutorFirstStageSteps
	if builder.postExecutorStage > 0 {
		stageSteps = builder.postExecutorSecondStageSteps
	}

	for _, step := range stageSteps {
		if err := step.execute(builder.postExecutorStepsContext); err != nil {
			log.V(0).Info("error: Execution of post execute step failed")
			return err
		}
	}

	return nil
}

// CreateBuildEnvironment constructs the environment variables to be provided to the assemble
// script and committed in the new image.
func CreateBuildEnvironment(sourcePath string, cfgEnv api.EnvironmentList) []string {
	s2iEnv, err := scripts.GetEnvironment(filepath.Join(sourcePath, constants.Source))
	if err != nil {
		log.V(3).Infof("No user environment provided (%v)", err)
	}

	return append(scripts.ConvertEnvironmentList(s2iEnv), scripts.ConvertEnvironmentList(cfgEnv)...)
}

// Exists determines if the current build supports incremental workflow.
// It checks if the previous image exists in the system and if so, then it
// verifies that the save-artifacts script is present.
func (builder *STI) Exists(config *api.Config) bool {
	if !config.Incremental {
		return false
	}

	policy := config.PreviousImagePullPolicy
	if len(policy) == 0 {
		policy = api.DefaultPreviousImagePullPolicy
	}

	tag := util.FirstNonEmpty(config.IncrementalFromTag, config.Tag)

	startTime := time.Now()
	result, err := dockerpkg.PullImage(tag, builder.incrementalDocker, policy)
	builder.result.BuildInfo.Stages = api.RecordStageAndStepInfo(builder.result.BuildInfo.Stages, api.StagePullImages, api.StepPullPreviousImage, startTime, time.Now())

	if err != nil {
		builder.result.BuildInfo.FailureReason = utilstatus.NewFailureReason(
			utilstatus.ReasonPullPreviousImageFailed,
			utilstatus.ReasonMessagePullPreviousImageFailed,
		)
		log.V(2).Infof("Unable to pull previously built image %q: %v", tag, err)
		return false
	}

	return result.Image != nil && builder.installedScripts[constants.SaveArtifacts]
}

// Save extracts and restores the build artifacts from the previous build to
// the current build.
func (builder *STI) Save(config *api.Config) (err error) {
	artifactTmpDir := filepath.Join(config.WorkingDir, "upload", "artifacts")
	if builder.result == nil {
		builder.result = &api.Result{}
	}

	if err = builder.fs.Mkdir(artifactTmpDir); err != nil {
		builder.result.BuildInfo.FailureReason = utilstatus.NewFailureReason(
			utilstatus.ReasonFSOperationFailed,
			utilstatus.ReasonMessageFSOperationFailed,
		)
		return err
	}

	image := util.FirstNonEmpty(config.IncrementalFromTag, config.Tag)

	outReader, outWriter := io.Pipe()
	errReader, errWriter := io.Pipe()
	log.V(1).Infof("Saving build artifacts from image %s to path %s", image, artifactTmpDir)
	extractFunc := func(string) error {
		startTime := time.Now()
		extractErr := builder.tar.ExtractTarStream(artifactTmpDir, outReader)
		io.Copy(ioutil.Discard, outReader) // must ensure reader from container is drained
		builder.result.BuildInfo.Stages = api.RecordStageAndStepInfo(builder.result.BuildInfo.Stages, api.StageRetrieve, api.StepRetrievePreviousArtifacts, startTime, time.Now())

		if extractErr != nil {
			builder.fs.RemoveDirectory(artifactTmpDir)
		}

		return extractErr
	}

	user := config.AssembleUser
	if len(user) == 0 {
		user, err = builder.docker.GetImageUser(image)
		if err != nil {
			builder.result.BuildInfo.FailureReason = utilstatus.NewFailureReason(
				utilstatus.ReasonGenericS2IBuildFailed,
				utilstatus.ReasonMessageGenericS2iBuildFailed,
			)
			return err
		}
		log.V(3).Infof("The assemble user is not set, defaulting to %q user", user)
	} else {
		log.V(3).Infof("Using assemble user %q to extract artifacts", user)
	}

	opts := dockerpkg.RunContainerOptions{
		Image:           image,
		User:            user,
		ExternalScripts: builder.externalScripts[constants.SaveArtifacts],
		ScriptsURL:      config.ScriptsURL,
		Destination:     config.Destination,
		PullImage:       false,
		Command:         constants.SaveArtifacts,
		Stdout:          outWriter,
		Stderr:          errWriter,
		OnStart:         extractFunc,
		NetworkMode:     string(config.DockerNetworkMode),
		CGroupLimits:    config.CGroupLimits,
		CapDrop:         config.DropCapabilities,
		Binds:           config.BuildVolumes,
		SecurityOpt:     config.SecurityOpt,
		AddHost:         config.AddHost,
	}

	dockerpkg.StreamContainerIO(errReader, nil, func(s string) { log.Info(s) })
	err = builder.docker.RunContainer(opts)
	if e, ok := err.(s2ierr.ContainerError); ok {
		err = s2ierr.NewSaveArtifactsError(image, e.Output, err)
	}

	builder.result.BuildInfo.FailureReason = utilstatus.NewFailureReason(
		utilstatus.ReasonGenericS2IBuildFailed,
		utilstatus.ReasonMessageGenericS2iBuildFailed,
	)
	return err
}

// Execute runs the specified STI script in the builder image.
func (builder *STI) Execute(command string, user string, config *api.Config) error {
	log.V(2).Infof("Using image name %s", config.BuilderImage)

	// Ensure that the builder image is present in the local Docker daemon.
	// The image should have been pulled when the strategy was created, so
	// this should be a quick inspect of the existing image. However, if
	// the image has been deleted since the strategy was created, this will ensure
	// it exists before executing a script on it.
	builder.docker.CheckAndPullImage(config.BuilderImage)

	// we can't invoke this method before (for example in New() method)
	// because of later initialization of config.WorkingDir
	builder.env = CreateBuildEnvironment(config.WorkingDir, config.Environment)

	errOutput := ""
	outReader, outWriter := io.Pipe()
	errReader, errWriter := io.Pipe()
	externalScripts := builder.externalScripts[command]
	// if LayeredBuild is called then all the scripts will be placed inside the image
	if config.LayeredBuild {
		externalScripts = false
	}

	opts := dockerpkg.RunContainerOptions{
		Image:  config.BuilderImage,
		Stdout: outWriter,
		Stderr: errWriter,
		// The PullImage is false because the PullImage function should be called
		// before we run the container
		PullImage:       false,
		ExternalScripts: externalScripts,
		ScriptsURL:      config.ScriptsURL,
		Destination:     config.Destination,
		Command:         command,
		Env:             builder.env,
		User:            user,
		PostExec:        builder.postExecutor,
		NetworkMode:     string(config.DockerNetworkMode),
		CGroupLimits:    config.CGroupLimits,
		CapDrop:         config.DropCapabilities,
		Binds:           config.BuildVolumes,
		SecurityOpt:     config.SecurityOpt,
		AddHost:         config.AddHost,
	}

	// If there are injections specified, override the original assemble script
	// and wait till all injections are uploaded into the container that runs the
	// assemble script.
	injectionError := make(chan error)
	if len(config.Injections) > 0 && command == constants.Assemble {
		workdir, err := builder.docker.GetImageWorkdir(config.BuilderImage)
		if err != nil {
			builder.result.BuildInfo.FailureReason = utilstatus.NewFailureReason(
				utilstatus.ReasonGenericS2IBuildFailed,
				utilstatus.ReasonMessageGenericS2iBuildFailed,
			)
			return err
		}
		config.Injections = util.FixInjectionsWithRelativePath(workdir, config.Injections)
		truncatedFiles, err := util.ListFilesToTruncate(builder.fs, config.Injections)
		if err != nil {
			builder.result.BuildInfo.FailureReason = utilstatus.NewFailureReason(
				utilstatus.ReasonInstallScriptsFailed,
				utilstatus.ReasonMessageInstallScriptsFailed,
			)
			return err
		}
		rmScript, err := util.CreateTruncateFilesScript(truncatedFiles, rmInjectionsScript)
		if len(rmScript) != 0 {
			defer os.Remove(rmScript)
		}
		if err != nil {
			builder.result.BuildInfo.FailureReason = utilstatus.NewFailureReason(
				utilstatus.ReasonGenericS2IBuildFailed,
				utilstatus.ReasonMessageGenericS2iBuildFailed,
			)
			return err
		}
		opts.CommandOverrides = func(cmd string) string {
			// If an s2i build has injections, the s2i container's main command must be altered to
			// do the following:
			//     1) Wait for the injections to be uploaded
			//     2) Check if there were any errors uploading the injections
			//     3) Run the injection removal script after `assemble` completes
			//
			// The injectionResultFile should always be uploaded to the s2i container after the
			// injected volumes are added. If this file is non-empty, it indicates that an error
			// occurred during the injection process and the s2i build should fail.
			return fmt.Sprintf("while [ ! -f %[1]q ]; do sleep 0.5; done; if [ -s %[1]q ]; then exit 1; fi; %[2]s; result=$?; . %[3]s; exit $result",
				injectionResultFile, cmd, rmInjectionsScript)
		}
		originalOnStart := opts.OnStart
		opts.OnStart = func(containerID string) error {
			defer close(injectionError)
			injectErr := builder.uploadInjections(config, rmScript, containerID)
			if err := builder.uploadInjectionResult(injectErr, containerID); err != nil {
				injectionError <- err
				return err
			}
			if originalOnStart != nil {
				return originalOnStart(containerID)
			}
			return nil
		}
	} else {
		close(injectionError)
	}

	if !config.LayeredBuild {
		r, w := io.Pipe()
		opts.Stdin = r

		go func() {
			// Wait for the injections to complete and check the error. Do not start
			// streaming the sources when the injection failed.
			if <-injectionError != nil {
				w.Close()
				return
			}
			log.V(2).Info("starting the source uploading ...")
			uploadDir := filepath.Join(config.WorkingDir, "upload")
			w.CloseWithError(builder.tar.CreateTarStream(uploadDir, false, w))
		}()
	}

	dockerpkg.StreamContainerIO(outReader, nil, func(s string) {
		if !config.Quiet {
			log.Info(strings.TrimSpace(s))
		}
	})

	c := dockerpkg.StreamContainerIO(errReader, &errOutput, func(s string) { log.Info(s) })

	err := builder.docker.RunContainer(opts)
	if err != nil {
		// Must wait for StreamContainerIO goroutine above to exit before reading errOutput.
		<-c

		if isMissingRequirements(errOutput) {
			err = errMissingRequirements
		} else if e, ok := err.(s2ierr.ContainerError); ok {
			err = s2ierr.NewContainerError(config.BuilderImage, e.ErrorCode, errOutput+e.Output)
		}
	}

	return err
}

// uploadInjections uploads the injected volumes to the s2i container, along with the source
// removal script to truncate volumes that should not be kept.
func (builder *STI) uploadInjections(config *api.Config, rmScript, containerID string) error {
	log.V(2).Info("starting the injections uploading ...")
	for _, s := range config.Injections {
		if err := builder.docker.UploadToContainer(builder.fs, s.Source, s.Destination, containerID); err != nil {
			return util.HandleInjectionError(s, err)
		}
	}
	if err := builder.docker.UploadToContainer(builder.fs, rmScript, rmInjectionsScript, containerID); err != nil {
		return util.HandleInjectionError(api.VolumeSpec{Source: rmScript, Destination: rmInjectionsScript}, err)
	}
	return nil
}

func (builder *STI) initPostExecutorSteps() {
	builder.postExecutorStepsContext = &postExecutorStepContext{}
	if len(builder.config.RuntimeImage) == 0 {
		builder.postExecutorFirstStageSteps = []postExecutorStep{
			&storePreviousImageStep{
				builder: builder,
				docker:  builder.docker,
			},
			&commitImageStep{
				image:   builder.config.BuilderImage,
				builder: builder,
				docker:  builder.docker,
				fs:      builder.fs,
				tar:     builder.tar,
			},
			&reportSuccessStep{
				builder: builder,
			},
			&removePreviousImageStep{
				builder: builder,
				docker:  builder.docker,
			},
		}
	} else {
		builder.postExecutorFirstStageSteps = []postExecutorStep{
			&downloadFilesFromBuilderImageStep{
				builder: builder,
				docker:  builder.docker,
				fs:      builder.fs,
				tar:     builder.tar,
			},
			&startRuntimeImageAndUploadFilesStep{
				builder: builder,
				docker:  builder.docker,
				fs:      builder.fs,
			},
		}
		builder.postExecutorSecondStageSteps = []postExecutorStep{
			&commitImageStep{
				image:   builder.config.RuntimeImage,
				builder: builder,
				docker:  builder.docker,
				tar:     builder.tar,
			},
			&reportSuccessStep{
				builder: builder,
			},
		}
	}
}

// uploadInjectionResult uploads a result file to the s2i container, indicating
// that the injections have completed. If a non-nil error is passed in, it is returned
// to ensure the error status of the injection upload is reported.
func (builder *STI) uploadInjectionResult(startErr error, containerID string) error {
	resultFile, err := util.CreateInjectionResultFile(startErr)
	if len(resultFile) > 0 {
		defer os.Remove(resultFile)
	}
	if err != nil {
		return err
	}
	err = builder.docker.UploadToContainer(builder.fs, resultFile, injectionResultFile, containerID)
	if err != nil {
		return util.HandleInjectionError(api.VolumeSpec{Source: resultFile, Destination: injectionResultFile}, err)
	}
	return startErr
}

func isMissingRequirements(text string) bool {
	tarCommand, _ := regexp.MatchString(`.*tar.*not found`, text)
	shCommand, _ := regexp.MatchString(`.*/bin/sh.*no such file or directory`, text)
	return tarCommand || shCommand
}

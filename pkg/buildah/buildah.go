package buildah

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"

	dockertypes "github.com/docker/docker/api/types"

	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/api/constants"
	"github.com/openshift/source-to-image/pkg/docker"
	s2ierr "github.com/openshift/source-to-image/pkg/errors"
	s2itar "github.com/openshift/source-to-image/pkg/tar"
	"github.com/openshift/source-to-image/pkg/util/fs"
	"github.com/openshift/source-to-image/pkg/util/interrupt"
	utillog "github.com/openshift/source-to-image/pkg/util/log"
)

var log = utillog.StderrLog

// Buildah implements docker.Docker interface using buildah as a backend.
type Buildah struct {
	client       docker.Client
	containerIDs map[string]string // map of image names and buildah from containerID
}

// Version returns a empty docker based version, not applicable to buildah.
func (b *Buildah) Version() (dockertypes.Version, error) {
	return dockertypes.Version{}, nil
}

// CheckReachable noop implementation, not applicable to buildah.
func (b *Buildah) CheckReachable() error {
	return nil
}

// InspectImage runs local "buildah inspect", but transforms the output into a api.Image instance. It
// can return error when the command does.
func (b *Buildah) InspectImage(name string) (*api.Image, error) {
	imageMetadata, err := InspectImage(name)
	if err != nil {
		log.V(4).Infof("error inspecting image %s: %v", name, err)
		return nil, s2ierr.NewInspectImageError(name, err)
	}
	return &api.Image{
		ID: imageMetadata.FromImageID,
		Config: &api.ContainerConfig{
			Env:    imageMetadata.Docker.Config.Env,
			Labels: imageMetadata.Docker.Config.Labels,
		},
	}, nil
}

// IsImageInLocalRegistry tries to inspect the image name, if no error is raised it returns true.
func (b *Buildah) IsImageInLocalRegistry(name string) (bool, error) {
	name = GetImageName(name)
	_, err := InspectImage(name)
	if err != nil {
		return false, err
	}
	return true, nil
}

// IsImageOnBuild return true when no on-build information is found.
func (b *Buildah) IsImageOnBuild(name string) bool {
	onBuild, err := b.GetOnBuild(name)
	return err == nil && len(onBuild) > 0
}

// GetOnBuild inspect image metadata to identify if image carries on-build labels.
func (b *Buildah) GetOnBuild(name string) ([]string, error) {
	name = docker.GetImageName(name)
	imageMetadata, err := InspectImage(name)
	if err != nil {
		return nil, err
	}
	return imageMetadata.Docker.Config.OnBuild, nil
}

// RemoveContainer by executing "buldah rm", and returns the error it may raise.
func (b *Buildah) RemoveContainer(id string) error {
	_, err := Execute([]string{"buildah", "rm", id}, nil, true)
	return err
}

// GetScriptsURL executes original docker.GetScriptsURL, and proxy the results.
func (b *Buildah) GetScriptsURL(name string) (string, error) {
	imageMetadata, err := b.CheckAndPullImage(name)
	if err != nil {
		return "", err
	}
	return docker.GetScriptsURL(imageMetadata), nil
}

// GetAssembleInputFiles inspect image labels to find AssembleInputFilesLabel, or return empty. It
// can return error in case of pulling and inspection errors.
func (b *Buildah) GetAssembleInputFiles(name string) (string, error) {
	imageMetadata, err := b.CheckAndPullImage(name)
	if err != nil {
		return "", err
	}
	label, exists := imageMetadata.Config.Labels[constants.AssembleInputFilesLabel]
	if !exists {
		log.V(0).Infof("warning: Image %q does not contain a value for the %s label",
			name, constants.AssembleInputFilesLabel)
	} else {
		log.V(3).Infof("Image %q contains %s set to %q",
			name, constants.AssembleInputFilesLabel, label)
	}
	return label, nil
}

// GetAssembleRuntimeUser inspect image labels to find AssembleRuntimeUserLabel, or return empty. It
// can return error in case of pulling and inspection errors.
func (b *Buildah) GetAssembleRuntimeUser(name string) (string, error) {
	imageMetadata, err := b.CheckAndPullImage(name)
	if err != nil {
		return "", err
	}
	label := imageMetadata.Config.Labels[constants.AssembleRuntimeUserLabel]
	return label, nil
}

// GetImageName proxy method to the same implementation on Docker.
func GetImageName(name string) string {
	return docker.GetImageName(name)
}

// GetImageID returns the image ID, using client actor. It's mostly used during tests. It can return
// error in case of inspect method does.
func (b *Buildah) GetImageID(name string) (string, error) {
	name = GetImageName(name)
	imageMetadata, err := InspectImage(name)
	if err != nil {
		return "", err
	}
	return imageMetadata.FromImageID, nil
}

// GetImageWorkdir used a post-execution step, will inspect image to gather workingDir attribute. It
// can return error based on inspect command.
func (b *Buildah) GetImageWorkdir(name string) (string, error) {
	imageMetadata, err := InspectImage(name)
	if err != nil {
		return "", err
	}
	workingDir := imageMetadata.Docker.Config.WorkingDir
	if workingDir == "" {
		workingDir = "/"
	}
	return workingDir, nil
}

// CommitContainer execute the final configuration of the container, adding entrypoint and other
// settings before commiting the image, using label "io.k8s.display-name" as the final image tag,
// when found among other labels. It can return errors related to running "buildah config" and
// "commit" sub-commands.
func (b *Buildah) CommitContainer(opts docker.CommitContainerOptions) (string, error) {
	containerID := opts.ContainerID
	configCmd := []string{"buildah", "config"}

	if opts.User != "" {
		configCmd = append(configCmd, []string{"--user", opts.User}...)
	}
	if len(opts.Entrypoint) > 0 {
		configCmd = append(configCmd, []string{
			"--entrypoint",
			fmt.Sprintf("[\"%s\"]", strings.Join(opts.Entrypoint, "\",\"")),
		}...)
	}
	for _, c := range opts.Command {
		configCmd = append(configCmd, []string{"--cmd", c}...)
	}
	for _, e := range opts.Env {
		configCmd = append(configCmd, []string{"--env", e}...)
	}

	var imageTag string
	for k, v := range opts.Labels {
		// extracting final image tag via labels
		if k == "io.k8s.display-name" {
			imageTag = v
		}
		configCmd = append(configCmd, []string{"--label", fmt.Sprintf("%s=%s", k, v)}...)
	}

	configCmd = append(configCmd, containerID)

	// executing container configuration command, saving image metadata
	_, err := Execute(configCmd, nil, true)
	if err != nil {
		return "", err
	}

	// executing container commit command, saving the actual container image, and keeping committed
	// container unique-id
	log.V(2).Infof("Commiting container '%s'...", containerID)
	commitCmd := []string{"buildah", "commit", "--quiet", containerID}
	if imageTag != "" {
		commitCmd = append(commitCmd, imageTag)
	}
	committedContainerIDBytes, err := Execute(commitCmd, nil, true)
	if err != nil {
		return "", err
	}
	committedContainerID := chompBytesToString(committedContainerIDBytes)
	log.V(2).Infof("Container ID '%s' committed successfully as '%s' image .",
		containerID, committedContainerID)
	return committedContainerID, nil
}

// RemoveImage execute buildah rmi in order to remove the informed image. It can return error when
// the command does.
func (b *Buildah) RemoveImage(containerID string) error {
	log.V(2).Infof("Removing image '%s'...", containerID)
	_, err := Execute([]string{"buildah", "rmi", "--force", containerID}, nil, true)
	return err
}

// CheckImage proxy to image inspection.
func (b *Buildah) CheckImage(name string) (*api.Image, error) {
	return b.InspectImage(name)
}

// PullImage execute "buildah pull" and inspect image in order to crate api.Image object. It can
// return error in case of buildah commands return error.
func (b *Buildah) PullImage(name string) (*api.Image, error) {
	log.V(2).Infof("Pulling image '%s'...", name)
	name = docker.GetImageName(name)
	if _, err := Execute([]string{"buildah", "pull", name}, nil, true); err != nil {
		return nil, err
	}
	return b.InspectImage(name)
}

// CheckAndPullImage make sure image exists or it can be pulled to local registry. It can return
// error when buildah commands fail, and in case of not being able to find informed image.
func (b *Buildah) CheckAndPullImage(name string) (*api.Image, error) {
	name = docker.GetImageName(name)
	displayName := name
	if !log.Is(3) {
		displayName = docker.ImageShortName(name)
	}

	image, err := b.CheckImage(name)
	if err != nil && !strings.Contains(err.Error(), "unable to get metadata") {
		return nil, err
	}
	if image == nil {
		log.V(1).Infof("Image %q not available locally, pulling ...", displayName)
		return b.PullImage(name)
	}

	log.V(3).Infof("Using locally available image %q", displayName)
	return image, nil
}

// BuildImage with "buildah bud" command, using path convention to find a Dockerfile, inside the same
// context directory.
func (b *Buildah) BuildImage(opts docker.BuildImageOptions) error {
	name := opts.Name
	contextDir := path.Join(opts.WorkingDir, "upload/src")
	log.V(0).Infof("Building Dockerfile on context directory '%s' using tag '%s'", contextDir, name)
	output, err := Execute([]string{"buildah", "bud", "--tag", name, contextDir}, nil, true)
	if output != nil {
		log.V(0).Infof("Build output:\n%s", output)
	}
	if err != nil {
		log.V(0).Infof("Error on building image: '%s'", err)
	}
	return err
}

// GetImageUser retrieve the user defined in the image, based on image metadata. It can return error
// on fetching metdata.
func (b *Buildah) GetImageUser(name string) (string, error) {
	name = docker.GetImageName(name)
	imageMetadata, err := InspectImage(name)
	if err != nil {
		return "", err
	}
	return imageMetadata.Docker.Config.User, nil
}

// GetImageEntrypoint inspect the image metadata to identify entrypoint as a slice. It can return
// error when inspect does.
func (b *Buildah) GetImageEntrypoint(name string) ([]string, error) {
	imageMetadata, err := InspectImage(name)
	if err != nil {
		return nil, err
	}
	return imageMetadata.Docker.Config.Entrypoint, nil
}

// GetLabels use image inspection to determine the labels, to be returned as a map. It can return
// error when issues to inspect image.
func (b *Buildah) GetLabels(name string) (map[string]string, error) {
	name = docker.GetImageName(name)
	imageMetadata, err := InspectImage(name)
	if err != nil {
		return nil, err
	}
	return imageMetadata.Docker.Config.Labels, nil
}

// From execute "buildah from" command and keep container ID stored as a instance attribute. This
// container ID should be reused in subsequent "buildah" calls, in order to compose a single image.
// It can return error when the command does.
func (b *Buildah) From(name string) (string, error) {
	log.V(2).Infof("Buildah FROM using '%s'...", name)

	containerIDBytes, err := Execute([]string{"buildah", "from", "--quiet", name}, nil, true)
	if err != nil {
		log.Error(err)
		return "", err
	}

	containerID := chompBytesToString(containerIDBytes)
	log.V(3).Infof("Buildah working container ID '%s' for image '%s'...", containerID, name)

	// storing container ID in local cache
	b.containerIDs[name] = containerID
	return containerID, nil
}

// uploadSourceCode will upload the informed directory using a tar stream in order to filter out
// ".s2iignore" entries, and the results are then "buildah copy" into a container.
func (b *Buildah) uploadSourceCode(uploadDir, destDir, container string) error {
	log.V(3).Infof("Uploading source-code directory from '%s' into %s's '%s'",
		uploadDir, container, destDir)

	fs := fs.NewFileSystem()
	th := s2itar.New(fs)

	r, w := io.Pipe()

	go func() {
		log.V(5).Infof("Creating tar stream from '%s' directory", uploadDir)
		err := th.CreateTarStream(uploadDir, false, w)
		if err == nil {
			if err = w.Close(); err != nil {
				log.Errorf("Error closing tar-stream writer: '%q'", err)
			}
		}
		_ = w.CloseWithError(err)
	}()

	tempDir, err := ioutil.TempDir("", fmt.Sprintf("s2i-upload-src-%s-", container))
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	log.V(3).Infof("Extracting tar stream on temporary directory '%s'", tempDir)
	if err = th.ExtractTarStream(tempDir, r); err != nil {
		return err
	}

	return b.UploadToContainer(fs, tempDir, destDir, container)
}

// prepareCmdTarDestination return the actual cmd and tar destination directory from informed Docker
// options. It will also upload the source code before assemble script, and can return error in this
// process.
func (b *Buildah) prepareCmdTarDestination(
	opts docker.RunContainerOptions,
	imageMetadata *api.Image,
	containerID string,
) ([]string, string, error) {
	// when is not the target image, in other words, when it's not suppose to run this image,
	// then contents will be copied into, and scripts will be ran against
	if opts.TargetImage && len(opts.CommandExplicit) > 0 {
		return nil, "", nil
	}

	tarDestination := docker.DetermineTarDestinationDir(opts, imageMetadata)
	log.V(3).Infof("Tar destination directory at '%s'", tarDestination)

	// when running assemble or usage commands, preparing and uploading the source-code to
	// the container image being build
	if opts.Command == constants.Assemble || opts.Command == constants.Usage {
		uploadDir := path.Join(opts.WorkingDir, "upload")
		log.V(2).Infof("Before assemble script, copying '%s' into container's '%s'",
			uploadDir, tarDestination)
		if err := b.uploadSourceCode(uploadDir, tarDestination, containerID); err != nil {
			return nil, "", err
		}
	}

	commandBaseDir := docker.DetermineCommandBaseDir(opts, imageMetadata, tarDestination)
	cmd := []string{path.Join(commandBaseDir, opts.Command)}
	log.V(5).Infof("Setting '%q' command for container ...", cmd)
	return cmd, tarDestination, nil
}

// RunContainer will construct the target container with buildah, handling on-start and
// post-execution steps.
func (b *Buildah) RunContainer(opts docker.RunContainerOptions) error {
	createOpts := opts.AsDockerCreateContainerOptions()
	image := createOpts.Config.Image

	// executing image pull, making sure it's on local registry
	if opts.PullImage {
		_, err := b.CheckAndPullImage(image)
		if err != nil {
			return err
		}
	}

	imageMetadata, err := b.InspectImage(image)
	if err != nil {
		return err
	}

	log.V(0).Infof("Creating application image based on '%q'", image)
	// starting a new image based on image informed
	containerID, err := b.From(image)
	if err != nil {
		return err
	}

	removeContainerFn := func() {
		log.V(3).Info("Removing container ID '%s'", containerID)
		if err := b.RemoveContainer(containerID); err != nil {
			log.V(0).Infof("warning: Failed to remove container %q: %v", containerID, err)
		} else {
			log.V(4).Infof("Removed container %q", containerID)
		}
	}

	return interrupt.New(docker.DumpStack, removeContainerFn).Run(func() error {
		cmd, tarDestination, err := b.prepareCmdTarDestination(opts, imageMetadata, containerID)

		// calling out for on-start before actual command, since using buildah it works in a sightly
		// different way than using Docker, using buildah the target container image is not being
		// executed therefore copying on-start contents before.
		onStartExecuted := false
		if opts.Command == constants.Assemble && opts.OnStart != nil {
			if err = opts.OnStart(containerID); err != nil {
				return err
			}
			onStartExecuted = true
		}

		if len(cmd) > 0 {
			// making sure the outputs are not shown during save artifacts
			verbose := true
			if opts.Command == constants.SaveArtifacts {
				verbose = false
			}

			// preparing buildah run command
			baseCmd := []string{"buildah", "run", containerID, "--"}
			runCmd := append(baseCmd, cmd...)

			var stdinReader io.Reader
			if opts.Stdin != nil {
				buffer := []byte{}
				if _, err = opts.Stdin.Read(buffer); err != nil {
					return err
				}
				stdinReader = bytes.NewReader(buffer)
			}

			stdout, err := Execute(runCmd, stdinReader, verbose)
			if err != nil {
				return err
			}

			if onStartExecuted {
				// after assemble, running injections clean up script with data injected in container
				// using "onStart" before
				if opts.Command == constants.Assemble && opts.RmInjectionsScript != "" {
					injectionScript := []string{"/bin/sh", opts.RmInjectionsScript}
					rmInjectionCmd := append(baseCmd, injectionScript...)
					if _, err = Execute(rmInjectionCmd, nil, verbose); err != nil {
						return err
					}
				}
			} else {
				// write the contents of buildah's run stdout, which is expected by opts.OnStart
				// current implementation once finished writing, it is required to close the pipe so
				// the reader knows no more data comes through
				go func() {
					if _, err = opts.Stdout.Write(stdout); err != nil {
						log.Error(err)
					}
					if err = opts.Stdout.Close(); err != nil {
						log.Error(err)
					}
				}()
			}
		}

		// regular on-start routine, if not executed before
		if opts.OnStart != nil && !onStartExecuted {
			if err = opts.OnStart(containerID); err != nil {
				return err
			}
		}

		if opts.PostExec != nil {
			log.V(2).Infof("Invoking PostExecute function(s)...")
			if err = opts.PostExec.PostExecute(containerID, tarDestination); err != nil {
				return err
			}
		}

		return nil
	})
}

// UploadToContainer is called back via "OnStart" action in RunContainer, therefore it can be simply
// translated as a "copy" call using buildah in the container image being build It can return error
// in case buildah command does.
func (b *Buildah) UploadToContainer(fs fs.FileSystem, srcPath, destPath, container string) error {
	log.V(3).Infof("Copying local '%s' into container's '%s' (container-id '%s')",
		srcPath, destPath, container)
	_, err := Execute([]string{"buildah", "copy", container, srcPath, destPath}, nil, true)
	return err
}

// UploadToContainerWithTarWriter using buildah it can be just redirected to UploadToContainer.
func (b *Buildah) UploadToContainerWithTarWriter(
	fs fs.FileSystem,
	srcPath string,
	destPath string,
	container string,
	makeTarWriter func(io.Writer) s2itar.Writer,
) error {
	return b.UploadToContainer(fs, srcPath, destPath, container)
}

// mount execute buildah mount on a container and return the local mount path employed. It can have
// errors when buildah command does.
func (b *Buildah) mount(container string) (string, error) {
	output, err := Execute([]string{"buildah", "unshare", "buildah", "mount", container}, nil, true)
	if err != nil {
		return "", err
	}
	mountPath := chompBytesToString(output)
	log.V(3).Infof("Container '%s' is mounted on '%s'", container, mountPath)
	return mountPath, nil
}

// unmount execute buildah unmount on container, and log eventual errors.
func (b *Buildah) unmount(container string) {
	log.V(3).Infof("Unmount container '%s' volumes", container)
	_, err := Execute([]string{"buildah", "unshare", "buildah", "unmount", container}, nil, true)
	if err != nil {
		log.Errorf("Error during unmount: '%q'", err)
	}
}

// DownloadFromContainer based on informed image, it will be mounted in the host, and mount path
// given by buildah is combined with containerPath. The final file or directory is transferred back
// to the host via tar archive. It can return errors during the buildah commands and during the
// tar archive.
func (b *Buildah) DownloadFromContainer(containerPath string, w io.Writer, container string) error {
	log.V(2).Infof("Downloading path '%s' from container '%s'...", containerPath, container)

	mountPath, err := b.mount(container)
	if err != nil {
		log.Error(err)
		return err
	}
	defer b.unmount(container)

	// full path of source file/directory, using buildah mount path
	srcPath := path.Join(mountPath, containerPath)

	// checking if file/directory exists
	_, err = os.Stat(srcPath)
	if os.IsNotExist(err) {
		log.V(3).Infof("warning: File is not found in container: '%s' (%q)", srcPath, err)
		return err
	}

	// transfering contents as a tar archive
	tarHandler := s2itar.New(fs.NewFileSystem())
	return tarHandler.CreateTarStream(srcPath, true, w)
}

// NewBuildah returns a new instance of Buildah.
func NewBuildah(client docker.Client) *Buildah {
	return &Buildah{
		client:       client,
		containerIDs: make(map[string]string),
	}
}

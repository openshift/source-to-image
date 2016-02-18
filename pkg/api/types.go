package api

import (
	"fmt"
	"path/filepath"
	"strings"

	docker "github.com/fsouza/go-dockerclient"

	"github.com/openshift/source-to-image/pkg/util/user"
)

// Image label namespace constants
const (
	DefaultNamespace    = "io.openshift.s2i."
	KubernetesNamespace = "io.k8s."
)

// invalidFilenameCharacters contains a list of character we consider malicious
// when injecting the directories into containers.
const invalidFilenameCharacters = `\:;*?"<>|%#$!+{}&[],"'` + "`"

const (
	// PullAlways means that we always attempt to pull the latest image.
	PullAlways PullPolicy = "always"

	// PullNever means that we never pull an image, but only use a local image.
	PullNever PullPolicy = "never"

	// PullIfNotPresent means that we pull if the image isn't present on disk.
	PullIfNotPresent PullPolicy = "if-not-present"

	// DefaultBuilderPullPolicy specifies the default pull policy to use
	DefaultBuilderPullPolicy = PullIfNotPresent

	// DefaultPreviousImagePullPolicy specifies policy for pulling the previously
	// build Docker image when doing incremental build
	DefaultPreviousImagePullPolicy = PullIfNotPresent
)

// Config contains essential fields for performing build.
type Config struct {
	// DisplayName is a result image display-name label. This defaults to the
	// output image name.
	DisplayName string

	// Description is a result image description label. The default is no
	// description.
	Description string

	// BuilderImage describes which image is used for building the result images.
	BuilderImage string

	// BuilderImageVersion provides optional version information about the builder image.
	BuilderImageVersion string

	// BuilderBaseImageVersion provides optional version information about the builder base image.
	BuilderBaseImageVersion string

	// DockerConfig describes how to access host docker daemon.
	DockerConfig *DockerConfig

	// DockerCfgPath provides the path to the .dockercfg file
	DockerCfgPath string

	// PullAuthentication holds the authentication information for pulling the
	// Docker images from private repositories
	PullAuthentication docker.AuthConfiguration

	// IncrementalAuthentication holds the authentication information for pulling the
	// previous image from private repositories
	IncrementalAuthentication docker.AuthConfiguration

	// DockerNetworkMode is used to set the docker network setting to --net=container:<id>
	// when the builder is invoked from a container.
	DockerNetworkMode DockerNetworkMode

	// PreserveWorkingDir describes if working directory should be left after processing.
	PreserveWorkingDir bool

	// DisableRecursive disables the --recursive option for the git clone that
	// allows to use the GIT without requiring the git submodule to be called.
	DisableRecursive bool

	// Source URL describing the location of sources used to build the result image.
	Source string

	// Ref is a tag/branch to be used for build.
	Ref string

	// Tag is a result image tag name.
	Tag string

	// BuilderPullPolicy specifies when to pull the builder image
	BuilderPullPolicy PullPolicy

	// PreviousImagePullPolicy specifies when to pull the previously build image
	// when doing incremental build
	PreviousImagePullPolicy PullPolicy

	// ForcePull defines if the builder image should be always pulled or not.
	// This is now deprecated by BuilderPullPolicy and will be removed soon.
	// Setting this to 'true' equals setting BuilderPullPolicy to 'PullAlways'.
	// Setting this to 'false' equals setting BuilderPullPolicy to 'PullIfNotPresent'
	ForcePull bool

	// Incremental describes whether to try to perform incremental build.
	Incremental bool

	// RemovePreviousImage describes if previous image should be removed after successful build.
	// This applies only to incremental builds.
	RemovePreviousImage bool

	// Environment is a map of environment variables to be passed to the image.
	Environment map[string]string

	// EnvironmentFile provides the path to a file with list of environment
	// variables.
	EnvironmentFile string

	// LabelNamespace provides the namespace under which the labels will be generated.
	LabelNamespace string

	// CallbackURL is a URL which is called upon successful build to inform about that fact.
	CallbackURL string

	// ScriptsURL is a URL describing the localization of STI scripts used during build process.
	ScriptsURL string

	// Destination specifies a location where the untar operation will place its artifacts.
	Destination string

	// WorkingDir describes temporary directory used for downloading sources, scripts and tar operations.
	WorkingDir string

	// WorkingSourceDir describes the subdirectory off of WorkingDir set up during the repo download
	// that is later used as the root for ignore processing
	WorkingSourceDir string

	// LayeredBuild describes if this is build which layered scripts and sources on top of BuilderImage.
	LayeredBuild bool

	// Operate quietly. Progress and assemble script output are not reported, only fatal errors.
	// (default: false).
	Quiet bool

	// Specify a relative directory inside the application repository that should
	// be used as a root directory for the application.
	ContextDir string

	// AllowedUIDs is a list of user ranges of users allowed to run the builder image.
	// If a range is specified and the builder image uses a non-numeric user or a user
	// that is outside the specified range, then the build fails.
	AllowedUIDs user.RangeList

	// AssembleUser specifies the user to run the assemble script in container
	AssembleUser string

	// RunImage will trigger a "docker run ..." invocation of the produced image so the user
	// can see if it operates as he would expect
	RunImage bool

	// Usage allows for properly shortcircuiting s2i logic when `s2i usage` is invoked
	Usage bool

	// Injections specifies a list source/destination folders that are injected to
	// the container that runs assemble.
	// All files we inject will be truncated after the assemble script finishes.
	Injections InjectionList

	// CGroupLimits describes the cgroups limits that will be applied to any containers
	// run by s2i.
	CGroupLimits *CGroupLimits
}

type CGroupLimits struct {
	MemoryLimitBytes int64
	CPUShares        int64
	CPUPeriod        int64
	CPUQuota         int64
	MemorySwap       int64
}

// InjectPath contains definition of source directory and the injection path.
type InjectPath struct {
	SourcePath     string
	DestinationDir string
}

// InjectionList contains list of InjectPath.
type InjectionList []InjectPath

// DockerConfig contains the configuration for a Docker connection.
type DockerConfig struct {
	// Endpoint is the docker network endpoint or socket
	Endpoint string

	// CertFile is the certificate file path for a TLS connection
	CertFile string

	// KeyFile is the key file path for a TLS connection
	KeyFile string

	// CAFile is the certificate authority file path for a TLS connection
	CAFile string
}

// Result structure contains information from build process.
type Result struct {

	// Success describes whether the build was successful.
	Success bool

	// Messages is a list of messages from build process.
	Messages []string

	// WorkingDir describes temporary directory used for downloading sources, scripts and tar operations.
	WorkingDir string

	// ImageID describes resulting image ID.
	ImageID string
}

// InstallResult structure describes the result of install operation
type InstallResult struct {

	// Script describes which script this result refers to
	Script string

	// URL describes from where the script was taken
	URL string

	// Downloaded describes if download operation happened, this will be true for
	// external scripts, but false for scripts from inside the image
	Downloaded bool

	// Installed describes if script was installed to upload directory
	Installed bool

	// Error describes last error encountered during install operation
	Error error
}

// SourceInfo stores information about the source code
type SourceInfo struct {
	// Ref represents a commit SHA-1, valid GIT branch name or a GIT tag
	// The output image will contain this information as 'io.openshift.build.commit.ref' label.
	Ref string

	// CommitID represents an arbitrary extended object reference in GIT as SHA-1
	// The output image will contain this information as 'io.openshift.build.commit.id' label.
	CommitID string

	// Date contains a date when the committer created the commit.
	// The output image will contain this information as 'io.openshift.build.commit.date' label.
	Date string

	// AuthorName contains the name of the author
	// The output image will contain this information (along with AuthorEmail) as 'io.openshift.build.commit.author' label.
	AuthorName string

	// AuthorEmail contains the e-mail of the author
	// The output image will contain this information (along with AuthorName) as 'io.openshift.build.commit.author' lablel.
	AuthorEmail string

	// CommitterName contains the name of the committer
	CommitterName string

	// CommitterEmail contains the e-mail of the committer
	CommitterEmail string

	// Message represents the first 80 characters from the commit message.
	// The output image will contain this information as 'io.openshift.build.commit.message' label.
	Message string

	// Location contains a valid URL to the original repository.
	// The output image will contain this information as 'io.openshift.build.source-location' label.
	Location string

	// ContextDir contains path inside the Location directory that
	// contains the application source code.
	// The output image will contain this information as 'io.openshift.build.source-context-dir'
	// label.
	ContextDir string
}

// CloneConfig specifies the options used when cloning the application source
// code.
type CloneConfig struct {
	Recursive bool
	Quiet     bool
}

// DockerNetworkMode specifies the network mode setting for the docker container
type DockerNetworkMode string

const (
	// DockerNetworkModeHost places the container in the default (host) network namespace.
	DockerNetworkModeHost DockerNetworkMode = "host"
	// DockerNetworkModeBridge instructs docker to create a network namespace for this container connected to the docker0 bridge via a veth-pair.
	DockerNetworkModeBridge DockerNetworkMode = "bridge"
	// DockerNetworkModeContainerPrefix is the string prefix used by NewDockerNetworkModeContainer.
	DockerNetworkModeContainerPrefix string = "container:"
)

// NewDockerNetworkModeContainer creates a DockerNetworkMode value which instructs docker to place the container in the network namespace of an existing container.
// It can be used, for instance, to place the s2i container in the network namespace of the infrastructure container of a k8s pod.
func NewDockerNetworkModeContainer(id string) DockerNetworkMode {
	return DockerNetworkMode(DockerNetworkModeContainerPrefix + id)
}

// PullPolicy specifies a type for the method used to retrieve the Docker image
type PullPolicy string

// String implements the String() function of pflags.Value so this can be used as
// command line parameter.
// This method is really used just to show the default value when printing help.
// It will not default the configuration.
func (p *PullPolicy) String() string {
	if len(string(*p)) == 0 {
		return string(DefaultBuilderPullPolicy)
	}
	return string(*p)
}

// Type implements the Type() function of pflags.Value interface
func (p *PullPolicy) Type() string {
	return "string"
}

// Set implements the Set() function of pflags.Value interface
// The valid options are "always", "never" or "if-not-present"
func (p *PullPolicy) Set(v string) error {
	switch v {
	case "always":
		*p = PullAlways
	case "never":
		*p = PullNever
	case "if-not-present":
		*p = PullIfNotPresent
	default:
		return fmt.Errorf("invalid value %q, valid values are: always, never or if-not-present")
	}
	return nil
}

// IsInvalidFilename verifies if the provided filename contains malicious
// characters.
func IsInvalidFilename(name string) bool {
	return strings.ContainsAny(name, invalidFilenameCharacters)
}

// Set implements the Set() function of pflags.Value interface.
// This function parses the string that contains source:destination pair.
// When the destination is not specified, the source get copied into current
// working directory in container.
func (il *InjectionList) Set(value string) error {
	mount := strings.Split(value, ":")
	switch len(mount) {
	case 0:
		return fmt.Errorf("invalid format, must be source:destination")
	case 1:
		mount = append(mount, "")
		fallthrough
	case 2:
		mount[0] = strings.Trim(mount[0], `"'`)
		mount[1] = strings.Trim(mount[1], `"'`)
	default:
		return fmt.Errorf("invalid source:path definition")
	}
	s := InjectPath{SourcePath: filepath.Clean(mount[0]), DestinationDir: filepath.Clean(mount[1])}
	if IsInvalidFilename(s.SourcePath) || IsInvalidFilename(s.DestinationDir) {
		return fmt.Errorf("invalid characters in filename: %q", value)
	}
	*il = append(*il, s)
	return nil
}

// String implements the String() function of pflags.Value interface.
func (il *InjectionList) String() string {
	result := []string{}
	for _, i := range *il {
		result = append(result, strings.Join([]string{i.SourcePath, i.DestinationDir}, ":"))
	}
	return strings.Join(result, ",")
}

// Type implements the Type() function of pflags.Value interface.
func (il *InjectionList) Type() string {
	return "string"
}

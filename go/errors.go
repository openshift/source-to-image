package sti

type StiError int

const (
	ErrDockerConnectionFailed StiError = iota
	ErrNoSuchBaseImage
	ErrNoSuchRuntimeImage
	ErrPullImageFailed
	ErrSaveArtifactsFailed
	ErrCreateDockerfileFailed
	ErrCreateContainerFailed
	ErrInvalidBuildMethod
	ErrBuildFailed
	ErrCommitContainerFailed
	ErrInvalidRef
)

func (s StiError) Error() string {
	switch s {
	case ErrDockerConnectionFailed:
		return "Couldn't connect to docker."
	case ErrNoSuchBaseImage:
		return "Couldn't find base image"
	case ErrNoSuchRuntimeImage:
		return "Couldn't find runtime image"
	case ErrPullImageFailed:
		return "Couldn't pull image"
	case ErrSaveArtifactsFailed:
		return "Error saving artifacts for incremental build"
	case ErrCreateDockerfileFailed:
		return "Error creating Dockerfile"
	case ErrCreateContainerFailed:
		return "Error creating container"
	case ErrInvalidBuildMethod:
		return "Invalid build method - valid methods are: run,build"
	case ErrBuildFailed:
		return "Running assemble in base image failed"
	case ErrCommitContainerFailed:
		return "Failed to commit built container"
	case ErrInvalidRef:
		return "Invalid git ref"
	default:
		return "Unknown error"
	}
}

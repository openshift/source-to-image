package errors

import (
	"fmt"
)

// Common STI errors
const (
	ErrInspectImage int = 1 + iota
	ErrPullImage
	ErrScriptDownload
	ErrSaveArtifacts
	ErrBuild
	ErrStiContainer
)

// Error represents an error thrown during STI execution
type Error struct {
	Message    string
	Details    error
	ErrorCode  int
	Suggestion string
}

// ContainerError is an error returned when a container exits with a non-zero code.
// ExitCode contains the exit code from the container
type ContainerError struct {
	Message    string
	Details    error
	ErrorCode  int
	Suggestion string
	ExitCode   int
}

// Error returns a string for a given error
func (s Error) Error() string {
	return s.Message
}

// Error returns a string for the given error
func (s ContainerError) Error() string {
	return s.Message
}

// NewInspectImageError returns a new error which indicates there was a problem
// inspecting the image
func NewInspectImageError(name string, err error) error {
	return Error{
		Message:    fmt.Sprintf("unable to get metadata for %s", name),
		Details:    err,
		ErrorCode:  ErrInspectImage,
		Suggestion: "check image name",
	}
}

// NewPullImageError returns a new error which indicates there was a problem
// pulling the image
func NewPullImageError(name string, err error) error {
	return Error{
		Message:    fmt.Sprintf("unable to get %s", name),
		Details:    err,
		ErrorCode:  ErrPullImage,
		Suggestion: "check image name",
	}
}

// NewScriptDownloadError returns a new error which indicates there was a problem
// downloading a script
func NewScriptDownloadError(name string, err error) error {
	return Error{
		Message:    fmt.Sprintf("%s script download failed", name),
		Details:    err,
		ErrorCode:  ErrScriptDownload,
		Suggestion: "provide URL with STI scripts with -s flag or check the image if it contains STI_SCRIPTS_URL variable set",
	}
}

// NewSaveArtifactsError returns a new error which indicates there was a problem
// calling save-artifacts script
func NewSaveArtifactsError(name string, err error) error {
	return Error{
		Message:    fmt.Sprintf("saving artifacts for %s failed", name),
		Details:    err,
		ErrorCode:  ErrSaveArtifacts,
		Suggestion: "check the save-artifacts script for errors",
	}
}

// NewBuildFailed returns a new error which indicates there was a problem
// building the image
func NewBuildError(name string, err error) error {
	return Error{
		Message:    fmt.Sprintf("building %s failed", name),
		Details:    err,
		ErrorCode:  ErrBuild,
		Suggestion: "check the assemble script for errors",
	}
}

// NewContainerError return a new error which indicates there was a problem
// invoking command inside container
func NewContainerError(name string, code int, err error) error {
	return ContainerError{
		Message:    fmt.Sprintf("non-zero (%d) exit code from %s", code, name),
		Details:    err,
		ErrorCode:  ErrStiContainer,
		Suggestion: "check the container logs for more information on the failure",
		ExitCode:   code,
	}
}

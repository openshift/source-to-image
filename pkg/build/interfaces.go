package build

import "github.com/openshift/source-to-image/pkg/api"

// Builder defines an arbitrary builder interface that performs the build
// based on the Request
type Builder interface {
	// Build executes the build based on Request and returns the Result
	Build(*api.Request) (*api.Result, error)
}

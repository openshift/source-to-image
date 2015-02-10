package sti

import (
	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/build"
)

// UsageHandler handles a request to display usage
type usageHandler interface {
	build.ScriptsHandler
	build.Preparer
	Cleanup(*api.Request)
	SetScripts([]api.Script, []api.Script)
}

// Usage display usage information about a particular build image
type Usage struct {
	handler usageHandler
	request *api.Request
}

// NewUsage creates a new instance of the default Usage implementation
func NewSTIUsage(req *api.Request) (*Usage, error) {
	b, err := NewSTI(req)
	if err != nil {
		return nil, err
	}
	return &Usage{b, req}, nil
}

// Show starts the builder container and invokes the usage script on it
// to print usage information for the script.
func (u *Usage) Show() error {
	b := u.handler
	defer b.Cleanup(u.request)

	b.SetScripts([]api.Script{api.Usage}, []api.Script{})

	if err := b.Prepare(u.request); err != nil {
		return err
	}

	return b.Execute(api.Usage, u.request)
}

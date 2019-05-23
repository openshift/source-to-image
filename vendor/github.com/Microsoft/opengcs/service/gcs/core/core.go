// Package core defines the interface representing the core functionality of a
// GCS-like program.
package core

import (
	"syscall"

	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/stdio"
)

// Core is the interface defining the core functionality of the GCS-like
// program. For a real implementation, this may include creating and configuring
// containers. However, it is also easily mocked out for testing.
type Core interface {
	CreateContainer(id string, info prot.VMHostedContainerSettings) error
	ExecProcess(id string, info prot.ProcessParameters, conSettings stdio.ConnectionSettings) (pid int, execInitErrorDone chan<- struct{}, err error)
	SignalContainer(id string, signal syscall.Signal) error
	SignalProcess(pid int, options prot.SignalProcessOptions) error
	GetProperties(id string, query string) (*prot.Properties, error)
	RunExternalProcess(info prot.ProcessParameters, conSettings stdio.ConnectionSettings) (pid int, err error)
	ModifySettings(id string, request *prot.ResourceModificationRequestResponse) error
	ResizeConsole(pid int, height, width uint16) error
	WaitContainer(id string) (func() prot.NotificationType, error)
	WaitProcess(pid int) (<-chan int, chan<- bool, error)
}

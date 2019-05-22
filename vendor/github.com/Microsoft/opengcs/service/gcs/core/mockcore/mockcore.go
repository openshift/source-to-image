// Package mockcore defines a mock implementation of the Core interface.
package mockcore

import (
	"sync"
	"syscall"

	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/stdio"
	"github.com/pkg/errors"
)

// Behavior describes the behavior of the mock core when a method is called.
type Behavior int

const (
	// Success specifies method calls should succeed.
	Success = iota
	// Error specifies method calls should return an error.
	Error
	// SingleSuccess specifies that the first method call should succeed and additional
	// calls should return an error.
	SingleSuccess
)

// CreateContainerCall captures the arguments of CreateContainer.
type CreateContainerCall struct {
	ID       string
	Settings prot.VMHostedContainerSettings
}

// ExecProcessCall captures the arguments of ExecProcess.
type ExecProcessCall struct {
	ID                 string
	Params             prot.ProcessParameters
	ConnectionSettings stdio.ConnectionSettings
}

// SignalContainerCall captures the arguments of SignalContainer.
type SignalContainerCall struct {
	ID     string
	Signal syscall.Signal
}

// SignalProcessCall captures the arguments of SignalProcess.
type SignalProcessCall struct {
	Pid     int
	Options prot.SignalProcessOptions
}

// GetPropertiesCall captures the arguments of GetProperties.
type GetPropertiesCall struct {
	ID    string
	Query string
}

// RunExternalProcessCall captures the arguments of RunExternalProcess.
type RunExternalProcessCall struct {
	Params             prot.ProcessParameters
	ConnectionSettings stdio.ConnectionSettings
}

// ModifySettingsCall captures the arguments of ModifySettings.
type ModifySettingsCall struct {
	ID      string
	Request *prot.ResourceModificationRequestResponse
}

// ResizeConsoleCall captures the arguments of ResizeConsole
type ResizeConsoleCall struct {
	Pid    int
	Height uint16
	Width  uint16
}

// WaitContainerCall captures the arguments of WaitContainer
type WaitContainerCall struct {
	ID string
}

// WaitProcessCall captures the arguments of WaitProcess
type WaitProcessCall struct {
	Pid int
}

// WaitProcessReturnContext captures the return context of a WaitProcess call.
type WaitProcessReturnContext struct {
	ExitCodeChan chan int
	DoneChan     chan bool
}

// MockCore serves as an argument capture mechanism which implements the Core
// interface. Arguments passed to one of its methods are stored to be queried
// later.
type MockCore struct {
	Behavior                     Behavior
	LastCreateContainer          CreateContainerCall
	LastExecProcess              ExecProcessCall
	LastSignalContainer          SignalContainerCall
	LastSignalProcess            SignalProcessCall
	LastGetProperties            GetPropertiesCall
	LastRunExternalProcess       RunExternalProcessCall
	LastModifySettings           ModifySettingsCall
	LastResizeConsole            ResizeConsoleCall
	LastWaitContainer            WaitContainerCall
	LastWaitProcess              WaitProcessCall
	LastWaitProcessReturnContext *WaitProcessReturnContext
	WaitContainerWg              sync.WaitGroup
}

// behaviorResulout produces the correct result given the MockCore's Behavior.
func (c *MockCore) behaviorResult() error {
	switch c.Behavior {
	case Success:
		return nil
	case Error:
		return errors.New("mockcore error")
	case SingleSuccess:
		c.Behavior = Error
		return nil
	default:
		return nil
	}
}

// CreateContainer captures its arguments.
func (c *MockCore) CreateContainer(id string, settings prot.VMHostedContainerSettings) error {
	c.LastCreateContainer = CreateContainerCall{
		ID:       id,
		Settings: settings,
	}
	return c.behaviorResult()
}

// ExecProcess captures its arguments and returns pid 101.
func (c *MockCore) ExecProcess(id string, params prot.ProcessParameters, conSettings stdio.ConnectionSettings) (pid int, execInitErrorDone chan<- struct{}, err error) {
	c.LastExecProcess = ExecProcessCall{
		ID:                 id,
		Params:             params,
		ConnectionSettings: conSettings,
	}
	return 101, nil, c.behaviorResult()
}

// SignalContainer captures its arguments.
func (c *MockCore) SignalContainer(id string, signal syscall.Signal) error {
	c.LastSignalContainer = SignalContainerCall{ID: id, Signal: signal}
	return c.behaviorResult()
}

// SignalProcess captures its arguments.
func (c *MockCore) SignalProcess(pid int, options prot.SignalProcessOptions) error {
	c.LastSignalProcess = SignalProcessCall{
		Pid:     pid,
		Options: options,
	}
	return c.behaviorResult()
}

// GetProperties captures its arguments. It then returns a properties with a
// process with pid 101.
func (c *MockCore) GetProperties(id string, query string) (*prot.Properties, error) {
	c.LastGetProperties = GetPropertiesCall{ID: id, Query: query}
	return &prot.Properties{
		ProcessList: []prot.ProcessDetails{prot.ProcessDetails{ProcessID: 101}},
	}, c.behaviorResult()
}

// RunExternalProcess captures its arguments and returns pid 101.
func (c *MockCore) RunExternalProcess(params prot.ProcessParameters, conSettings stdio.ConnectionSettings) (pid int, err error) {
	c.LastRunExternalProcess = RunExternalProcessCall{
		Params:             params,
		ConnectionSettings: conSettings,
	}
	return 101, c.behaviorResult()
}

// ModifySettings captures its arguments.
func (c *MockCore) ModifySettings(id string, request *prot.ResourceModificationRequestResponse) error {
	c.LastModifySettings = ModifySettingsCall{
		ID:      id,
		Request: request,
	}
	return c.behaviorResult()
}

// ResizeConsole captures its arguments and returns a nil error.
func (c *MockCore) ResizeConsole(pid int, height, width uint16) error {
	c.LastResizeConsole = ResizeConsoleCall{
		Pid:    pid,
		Height: height,
		Width:  width,
	}
	return c.behaviorResult()
}

// WaitContainer captures its arguments and returns a nil error.
func (c *MockCore) WaitContainer(id string) (func() prot.NotificationType, error) {
	c.LastWaitContainer = WaitContainerCall{
		ID: id,
	}
	c.WaitContainerWg.Done()
	return func() prot.NotificationType { return prot.NtUnexpectedExit }, c.behaviorResult()
}

// WaitProcess captures its arguments and returns a nil error.
func (c *MockCore) WaitProcess(pid int) (<-chan int, chan<- bool, error) {
	c.LastWaitProcess = WaitProcessCall{
		Pid: pid,
	}

	// All the tests to create one on their own but if one doesnt
	// exit make a default one.
	if c.LastWaitProcessReturnContext == nil {
		c.LastWaitProcessReturnContext = &WaitProcessReturnContext{}
	}

	return c.LastWaitProcessReturnContext.ExitCodeChan, c.LastWaitProcessReturnContext.DoneChan, c.behaviorResult()
}

// Package mockruntime defines a mock implementation of the Runtime interface.
package mockruntime

import (
	"sync"
	"syscall"

	"github.com/Microsoft/opengcs/service/gcs/runtime"
	"github.com/Microsoft/opengcs/service/gcs/stdio"
	oci "github.com/opencontainers/runtime-spec/specs-go"
)

// mockRuntime is an implementation of the Runtime interface which uses runC as
// the container runtime.
type mockRuntime struct {
	killed *sync.Cond
}

var _ runtime.Runtime = &mockRuntime{}

// NewRuntime constructs a new mockRuntime with the default settings.
func NewRuntime(_ string) runtime.Runtime {
	var lock sync.Mutex
	return &mockRuntime{killed: sync.NewCond(&lock)}
}

type container struct {
	id string
	r  *mockRuntime
}

func (r *mockRuntime) CreateContainer(id string, bundlePath string, stdioSet *stdio.ConnectionSet) (c runtime.Container, err error) {
	return &container{id: id, r: r}, nil
}

func (c *container) Start() error {
	return nil
}

func (c *container) ID() string {
	return c.id
}

func (c *container) Pid() int {
	return 101
}

func (c *container) Tty() *stdio.TtyRelay {
	return nil
}

func (c *container) PipeRelay() *stdio.PipeRelay {
	return nil
}

func (c *container) ExecProcess(process *oci.Process, stdioSet *stdio.ConnectionSet) (p runtime.Process, err error) {
	return c, nil
}

func (c *container) Kill(signal syscall.Signal) error {
	c.r.killed.L.Lock()
	defer c.r.killed.L.Unlock()
	c.r.killed.Broadcast()
	return nil
}

func (c *container) Delete() error {
	return nil
}

func (c *container) Pause() error {
	return nil
}

func (c *container) Resume() error {
	return nil
}

func (c *container) GetState() (*runtime.ContainerState, error) {
	state := &runtime.ContainerState{
		OCIVersion: "v1",
		ID:         "abcdef",
		Pid:        123,
		BundlePath: "/path/to/bundle",
		RootfsPath: "/path/to/rootfs",
		Status:     "running",
		Created:    "tuesday",
	}
	return state, nil
}

func (c *container) Exists() (bool, error) {
	return true, nil
}

func (r *mockRuntime) ListContainerStates() ([]runtime.ContainerState, error) {
	states := []runtime.ContainerState{
		runtime.ContainerState{
			OCIVersion: "v1",
			ID:         "abcdef",
			Pid:        123,
			BundlePath: "/path/to/bundle",
			RootfsPath: "/path/to/rootfs",
			Status:     "running",
			Created:    "tuesday",
		},
	}
	return states, nil
}

func (c *container) GetRunningProcesses() ([]runtime.ContainerProcessState, error) {
	states := []runtime.ContainerProcessState{
		runtime.ContainerProcessState{
			Pid:              123,
			Command:          []string{"cat", "file"},
			CreatedByRuntime: true,
			IsZombie:         true,
		},
	}
	return states, nil
}

func (c *container) GetAllProcesses() ([]runtime.ContainerProcessState, error) {
	states := []runtime.ContainerProcessState{
		runtime.ContainerProcessState{
			Pid:              123,
			Command:          []string{"cat", "file"},
			CreatedByRuntime: true,
			IsZombie:         true,
		},
	}
	return states, nil
}

func (c *container) Wait() (int, error) {
	c.r.killed.L.Lock()
	defer c.r.killed.L.Unlock()
	c.r.killed.Wait()
	return 123, nil
}

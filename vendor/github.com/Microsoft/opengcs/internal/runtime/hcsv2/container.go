// +build linux

package hcsv2

import (
	"context"
	"sync"
	"syscall"

	"github.com/Microsoft/opengcs/internal/oc"
	"github.com/Microsoft/opengcs/service/gcs/gcserr"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/runtime"
	"github.com/Microsoft/opengcs/service/gcs/stdio"
	"github.com/Microsoft/opengcs/service/gcs/transport"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"go.opencensus.io/trace"
)

type Container struct {
	id    string
	vsock transport.Transport

	spec *oci.Spec

	container   runtime.Container
	initProcess *Process

	etL      sync.Mutex
	exitType prot.NotificationType

	processesMutex sync.Mutex
	processesWg    sync.WaitGroup
	processes      map[uint32]*Process
}

func (c *Container) Start(ctx context.Context, conSettings stdio.ConnectionSettings) (_ int, err error) {
	_, span := trace.StartSpan(ctx, "opengcs::Container::Start")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", c.id))

	stdioSet, err := stdio.Connect(c.vsock, conSettings)
	if err != nil {
		return -1, err
	}
	if c.initProcess.spec.Terminal {
		ttyr := c.container.Tty()
		ttyr.ReplaceConnectionSet(stdioSet)
		ttyr.Start()
	} else {
		pr := c.container.PipeRelay()
		pr.ReplaceConnectionSet(stdioSet)
		pr.CloseUnusedPipes()
		pr.Start()
	}
	err = c.container.Start()
	if err != nil {
		stdioSet.Close()
	}
	return int(c.initProcess.pid), err
}

func (c *Container) ExecProcess(ctx context.Context, process *oci.Process, conSettings stdio.ConnectionSettings) (_ int, err error) {
	_, span := trace.StartSpan(ctx, "opengcs::Container::ExecProcess")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", c.id))

	stdioSet, err := stdio.Connect(c.vsock, conSettings)
	if err != nil {
		return -1, err
	}

	// Increment the waiters before the call so that WaitContainer cannot complete in a race
	// with adding a new process. When the process exits it will decrement this count.
	c.processesMutex.Lock()
	c.processesWg.Add(1)
	c.processesMutex.Unlock()

	p, err := c.container.ExecProcess(process, stdioSet)
	if err != nil {
		// We failed to exec any process. Remove our early count increment.
		c.processesMutex.Lock()
		c.processesWg.Done()
		c.processesMutex.Unlock()
		stdioSet.Close()
		return -1, err
	}

	pid := p.Pid()
	c.processesMutex.Lock()
	c.processes[uint32(pid)] = newProcess(c, process, p, uint32(pid), false)
	c.processesMutex.Unlock()
	return pid, nil
}

// GetProcess returns the *Process with the matching 'pid'. If the 'pid' does
// not exit returns error.
func (c *Container) GetProcess(pid uint32) (*Process, error) {
	if c.initProcess.pid == pid {
		return c.initProcess, nil
	}

	c.processesMutex.Lock()
	defer c.processesMutex.Unlock()

	p, ok := c.processes[pid]
	if !ok {
		return nil, gcserr.NewHresultError(gcserr.HrErrNotFound)
	}
	return p, nil
}

// GetAllProcessPids returns all process pids in the container namespace.
func (c *Container) GetAllProcessPids(ctx context.Context) (_ []int, err error) {
	_, span := trace.StartSpan(ctx, "opengcs::Container::GetAllProcessPids")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", c.id))

	state, err := c.container.GetAllProcesses()
	if err != nil {
		return nil, err
	}
	pids := make([]int, len(state))
	for i, s := range state {
		pids[i] = s.Pid
	}
	return pids, nil
}

// Kill sends 'signal' to the container process.
func (c *Container) Kill(ctx context.Context, signal syscall.Signal) (err error) {
	_, span := trace.StartSpan(ctx, "opengcs::Container::Kill")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute("cid", c.id),
		trace.Int64Attribute("signal", int64(signal)))

	err = c.container.Kill(signal)
	if err != nil {
		return err
	}
	c.setExitType(signal)
	return nil
}

// Wait waits for all processes exec'ed to finish as well as the init process
// representing the container.
func (c *Container) Wait() prot.NotificationType {
	_, span := trace.StartSpan(context.Background(), "opengcs::Container::Wait")
	defer span.End()
	span.AddAttributes(trace.StringAttribute("cid", c.id))

	c.processesWg.Wait()
	c.etL.Lock()
	defer c.etL.Unlock()
	return c.exitType
}

// setExitType sets `c.exitType` to the appropriate value based on `signal` if
// `signal` will take down the container.
func (c *Container) setExitType(signal syscall.Signal) {
	c.etL.Lock()
	defer c.etL.Unlock()

	if signal == syscall.SIGTERM {
		c.exitType = prot.NtGracefulExit
	} else if signal == syscall.SIGKILL {
		c.exitType = prot.NtForcedExit
	}
}

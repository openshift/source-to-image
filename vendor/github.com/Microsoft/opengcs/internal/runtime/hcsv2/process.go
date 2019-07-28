// +build linux

package hcsv2

import (
	"context"
	"fmt"
	"sync"
	"syscall"

	"github.com/Microsoft/opengcs/internal/log"
	"github.com/Microsoft/opengcs/internal/oc"
	"github.com/Microsoft/opengcs/service/gcs/gcserr"
	"github.com/Microsoft/opengcs/service/gcs/runtime"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"go.opencensus.io/trace"
)

// Process is a struct that defines the lifetime and operations associated with
// an oci.Process.
type Process struct {
	// c is the owning container
	c    *Container
	spec *oci.Process
	// cid is the container id that owns this process.
	cid string

	process runtime.Process
	pid     uint32
	// init is `true` if this is the container process itself
	init bool

	// This is only valid post the exitWg
	exitCode int
	exitWg   sync.WaitGroup

	// Used to allow addtion/removal to the writersWg after an initial wait has
	// already been issued. It is not safe to call Add/Done without holding this
	// lock.
	writersSyncRoot sync.Mutex
	// Used to track the number of writers that need to finish
	// before the process can be marked for cleanup.
	writersWg sync.WaitGroup
	// Used to track the 1st caller to the writersWg that successfully
	// acknowledges it wrote the exit response.
	writersCalled bool
}

// newProcess returns a Process struct that has been initialized with an
// outstanding wait for process exit, and post exit an outstanding wait for
// process cleanup to release all resources once at least 1 waiter has
// successfully written the exit response.
func newProcess(c *Container, spec *oci.Process, process runtime.Process, pid uint32, init bool) *Process {
	p := &Process{
		c:       c,
		spec:    spec,
		process: process,
		init:    init,
		cid:     c.id,
		pid:     pid,
	}
	p.exitWg.Add(1)
	p.writersWg.Add(1)
	go func() {
		ctx, span := trace.StartSpan(context.Background(), "newProcess::waitBackground")
		defer span.End()
		span.AddAttributes(
			trace.StringAttribute("cid", p.cid),
			trace.Int64Attribute("pid", int64(p.pid)))

		// Wait for the process to exit
		exitCode, err := p.process.Wait()
		if err != nil {
			log.G(ctx).WithError(err).Error("failed to wait for runc process")
		}
		p.exitCode = exitCode
		log.G(ctx).WithField("exitCode", p.exitCode).Debug("process exited")

		// Free any process waiters
		p.exitWg.Done()
		// Decrement any container process count waiters
		c.processesMutex.Lock()
		c.processesWg.Done()
		c.processesMutex.Unlock()

		// Schedule the removal of this process object from the map once at
		// least one waiter has read the result
		go func() {
			p.writersWg.Wait()
			c.processesMutex.Lock()

			_, span := trace.StartSpan(context.Background(), "newProcess::waitBackground::waitAllWaiters")
			defer span.End()
			span.AddAttributes(
				trace.StringAttribute("cid", p.cid),
				trace.Int64Attribute("pid", int64(p.pid)))

			delete(c.processes, p.pid)
			c.processesMutex.Unlock()
		}()
	}()
	return p
}

// Kill sends 'signal' to the process.
//
// If the process has already exited returns `gcserr.HrErrNotFound` by contract.
func (p *Process) Kill(ctx context.Context, signal syscall.Signal) (err error) {
	_, span := trace.StartSpan(ctx, "opengcs::Process::Kill")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute("cid", p.cid),
		trace.Int64Attribute("pid", int64(p.pid)),
		trace.Int64Attribute("signal", int64(signal)))

	if err := syscall.Kill(int(p.pid), signal); err != nil {
		if err == syscall.ESRCH {
			return gcserr.NewHresultError(gcserr.HrErrNotFound)
		}
		return err
	}
	if p.init {
		p.c.setExitType(signal)
	}
	return nil
}

// ResizeConsole resizes the tty to `height`x`width` for the process.
func (p *Process) ResizeConsole(ctx context.Context, height, width uint16) (err error) {
	_, span := trace.StartSpan(ctx, "opengcs::Process::ResizeConsole")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(
		trace.StringAttribute("cid", p.cid),
		trace.Int64Attribute("pid", int64(p.pid)),
		trace.Int64Attribute("height", int64(height)),
		trace.Int64Attribute("width", int64(width)))

	tty := p.process.Tty()
	if tty == nil {
		return fmt.Errorf("pid: %d, is not a tty and cannot be resized", p.pid)
	}
	return tty.ResizeConsole(height, width)
}

// Wait returns a channel that can be used to wait for the process to exit and
// gather the exit code. The second channel must be signaled from the caller
// when the caller has completed its use of this call to Wait.
func (p *Process) Wait() (<-chan int, chan<- bool) {
	ctx, span := trace.StartSpan(context.Background(), "opengcs::Process::Wait")
	span.AddAttributes(
		trace.StringAttribute("cid", p.cid),
		trace.Int64Attribute("pid", int64(p.pid)))

	exitCodeChan := make(chan int, 1)
	doneChan := make(chan bool)

	// Increment our waiters for this waiter
	p.writersSyncRoot.Lock()
	p.writersWg.Add(1)
	p.writersSyncRoot.Unlock()

	go func() {
		bgExitCodeChan := make(chan int, 1)
		go func() {
			p.exitWg.Wait()
			bgExitCodeChan <- p.exitCode
		}()

		// Wait for the exit code or the caller to stop waiting.
		select {
		case exitCode := <-bgExitCodeChan:
			exitCodeChan <- exitCode

			// The caller got the exit code. Wait for them to tell us they have
			// issued the write
			select {
			case <-doneChan:
				p.writersSyncRoot.Lock()
				// Decrement this waiter
				log.G(ctx).Debug("wait completed, releasing wait count")

				p.writersWg.Done()
				if !p.writersCalled {
					// We have at least 1 response for the exit code for this
					// process. Decrement the release waiter that will free the
					// process resources when the writersWg hits 0
					log.G(ctx).Debug("first wait completed, releasing first wait count")

					p.writersCalled = true
					p.writersWg.Done()
				}
				p.writersSyncRoot.Unlock()
				span.End()
			}

		case <-doneChan:
			// In this case the caller timed out before the process exited. Just
			// decrement the waiter but since no exit code we just deal with our
			// waiter.
			p.writersSyncRoot.Lock()
			log.G(ctx).Debug("wait canceled before exit, releasing wait count")

			p.writersWg.Done()
			p.writersSyncRoot.Unlock()
			span.End()
		}
	}()
	return exitCodeChan, doneChan
}

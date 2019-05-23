// +build linux

package hcsv2

import (
	"encoding/json"
	"os/exec"
	"strconv"
	"sync"
	"syscall"

	"github.com/Microsoft/opengcs/internal/network"
	"github.com/Microsoft/opengcs/service/gcs/gcserr"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/runtime"
	"github.com/Microsoft/opengcs/service/gcs/stdio"
	"github.com/Microsoft/opengcs/service/gcs/transport"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
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

func (c *Container) Start(conSettings stdio.ConnectionSettings) (int, error) {
	logrus.WithFields(logrus.Fields{
		"cid": c.id,
	}).Info("opengcs::Container::Start")

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

func (c *Container) ExecProcess(process *oci.Process, conSettings stdio.ConnectionSettings) (int, error) {
	logrus.WithFields(logrus.Fields{
		"cid": c.id,
	}).Info("opengcs::Container::ExecProcess")

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
	logrus.WithFields(logrus.Fields{
		"cid": c.id,
		"pid": pid,
	}).Info("opengcs::Container::GetProcess")

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
func (c *Container) GetAllProcessPids() ([]int, error) {
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
func (c *Container) Kill(signal syscall.Signal) error {
	logrus.WithFields(logrus.Fields{
		"cid":    c.id,
		"signal": signal,
	}).Info("opengcs::Container::Kill")

	err := c.container.Kill(signal)
	if err != nil {
		return err
	}
	c.setExitType(signal)
	return nil
}

// Wait waits for all processes exec'ed to finish as well as the init process
// representing the container.
func (c *Container) Wait() prot.NotificationType {
	logrus.WithFields(logrus.Fields{
		"cid": c.id,
	}).Info("opengcs::Container::Wait")

	c.processesWg.Wait()
	c.etL.Lock()
	defer c.etL.Unlock()
	return c.exitType
}

// AddNetworkAdapter adds `a` to the network namespace held by this container.
func (c *Container) AddNetworkAdapter(a *prot.NetworkAdapterV2) error {
	log := logrus.WithFields(logrus.Fields{
		"cid":               c.id,
		"adapterInstanceID": a.ID,
	})
	log.Info("opengcs::Container::AddNetworkAdapter")

	// TODO: netnscfg is not coded for v2 but since they are almost the same
	// just convert the parts of the adapter here.
	v1Adapter := &prot.NetworkAdapter{
		NatEnabled:         a.IPAddress != "",
		AllocatedIPAddress: a.IPAddress,
		HostIPAddress:      a.GatewayAddress,
		HostIPPrefixLength: a.PrefixLength,
		EnableLowMetric:    a.EnableLowMetric,
		EncapOverhead:      a.EncapOverhead,
	}

	cfg, err := json.Marshal(v1Adapter)
	if err != nil {
		return errors.Wrap(err, "failed to marshal adapter struct to JSON")
	}

	ifname, err := network.InstanceIDToName(a.ID, true)
	if err != nil {
		return err
	}

	out, err := exec.Command("netnscfg",
		"-if", ifname,
		"-nspid", strconv.Itoa(c.container.Pid()),
		"-cfg", string(cfg)).CombinedOutput()
	if err != nil {
		return errors.Wrapf(err, "failed to configure adapter cid: %s, aid: %s, if id: %s %s", c.id, a.ID, ifname, string(out))
	}
	return nil
}

// RemoveNetworkAdapter removes the network adapter `id` from the network
// namespace held by this container.
func (c *Container) RemoveNetworkAdapter(id string) error {
	logrus.WithFields(logrus.Fields{
		"cid":               c.id,
		"adapterInstanceID": id,
	}).Info("opengcs::Container::RemoveNetworkAdapter")

	// TODO: JTERRY75 - Implement removal if we ever need to support hot remove.
	logrus.WithFields(logrus.Fields{
		"cid":               c.id,
		"adapterInstanceID": id,
	}).Warning("opengcs::Container::RemoveNetworkAdapter - Not implemented")
	return nil
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

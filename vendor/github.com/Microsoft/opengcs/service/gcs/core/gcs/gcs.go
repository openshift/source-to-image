// Package gcs defines the core functionality of the GCS. This includes all
// the code which manages container and their state, including interfacing with
// the container runtime, forwarding container stdio through
// transport.Connections, and configuring networking for a container.
package gcs

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/Microsoft/opengcs/internal/storage"
	"github.com/Microsoft/opengcs/internal/storage/plan9"
	"github.com/Microsoft/opengcs/internal/storage/scsi"
	"github.com/Microsoft/opengcs/service/gcs/core"
	"github.com/Microsoft/opengcs/service/gcs/gcserr"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/runtime"
	"github.com/Microsoft/opengcs/service/gcs/stdio"
	"github.com/Microsoft/opengcs/service/gcs/transport"
	shellwords "github.com/mattn/go-shellwords"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// gcsCore is an implementation of the Core interface, defining the
// functionality of the GCS.
type gcsCore struct {
	// Rtime is the Runtime interface used by the GCS core.
	Rtime runtime.Runtime

	// vsock is the transport used to connect to plan9 servers.
	vsock transport.Transport

	containerCacheMutex sync.RWMutex
	// containerCache stores information about containers which persists
	// between calls into the gcsCore. It is structured as a map from container
	// ID to cache entry.
	containerCache map[string]*containerCacheEntry

	processCacheMutex sync.RWMutex
	// processCache stores information about processes which persists between calls
	// into the gcsCore. It is structured as a map from pid to cache entry.
	processCache map[int]*processCacheEntry

	// baseLogPath is the path where all container logs should be nested.
	baseLogPath string

	// baseStoragePath is the path where all container storage should be nested.
	baseStoragePath string

	// containerIndexMutex is to lock access to the containerIndex slice
	containerIndexMutex sync.Mutex

	// containerIndex is a slice that tracks the index that the container was
	// created in based on its array location and is aggressively reused when a
	// location is no longer needed.
	containerIndex []string
}

// getOrAddContainerIndex gets the index that a container was already inserted
// at or inserts the container at the next available index. This method will
// return an error if no available indexes could be found.
func (c *gcsCore) getOrAddContainerIndex(id string) (uint32, error) {
	c.containerIndexMutex.Lock()
	defer c.containerIndexMutex.Unlock()

	// len() returns int32 so we cannot index a value greater than that size.
	// And we know that given index range {0, MaxInt32 - 1} that if there are no
	// slots and a maximum slice size insertAt will be MaxInt32 at the end of
	// the loop.
	len := len(c.containerIndex)
	insertAt := len
	for i := 0; i < len; i++ {
		if c.containerIndex[i] == "" && i <= insertAt {
			insertAt = i
		} else if c.containerIndex[i] == id {
			// We already have inserted this id. Return its index.
			return uint32(i), nil
		}
	}

	if insertAt == math.MaxInt32 {
		return 0, fmt.Errorf("Maximum number (%d) of container indexes hit", math.MaxInt32)
	}

	if insertAt < len {
		c.containerIndex[insertAt] = id
	} else {
		c.containerIndex = append(c.containerIndex, id)
	}

	return uint32(insertAt), nil
}

// getContainerIDFromIndex gets the ID of the container registered at a given
// index. If the index is larger than the known ID's or a valid index that does
// not have a registered ID it returns "".
func (c *gcsCore) getContainerIDFromIndex(index uint32) string {
	c.containerIndexMutex.Lock()
	defer c.containerIndexMutex.Unlock()

	if int(index) < len(c.containerIndex) {
		return c.containerIndex[index]
	}

	return ""
}

// removeContainerIndex removes a container index in the list. This is safe to
// call multiple times as it does not affect the list if not found.
func (c *gcsCore) removeContainerIndex(id string) {
	c.containerIndexMutex.Lock()
	defer c.containerIndexMutex.Unlock()

	for i := 0; i < len(c.containerIndex); i++ {
		if c.containerIndex[i] == id {
			c.containerIndex[i] = ""
			return
		}
	}
}

// NewGCSCore creates a new gcsCore struct initialized with the given Runtime.
func NewGCSCore(baseLogPath, baseStoragePath string, rtime runtime.Runtime, vsock transport.Transport) core.Core {
	return &gcsCore{
		baseLogPath:     baseLogPath,
		baseStoragePath: baseStoragePath,
		Rtime:           rtime,
		vsock:           vsock,
		containerCache:  make(map[string]*containerCacheEntry),
		processCache:    make(map[int]*processCacheEntry),
	}
}

// containerCacheEntry stores cached information for a single container.
type containerCacheEntry struct {
	ID string
	// Index is the shortened storage location index for this container. It
	// represents the index in which this container was given on create.
	Index              uint32
	MappedVirtualDisks map[uint8]prot.MappedVirtualDisk
	MappedDirectories  map[uint32]prot.MappedDirectory
	NetworkAdapters    []prot.NetworkAdapter
	container          runtime.Container
	hasRunInitProcess  bool
	initProcess        *processCacheEntry
}

func newContainerCacheEntry(id string) *containerCacheEntry {
	return &containerCacheEntry{
		ID:                 id,
		MappedVirtualDisks: make(map[uint8]prot.MappedVirtualDisk),
		MappedDirectories:  make(map[uint32]prot.MappedDirectory),
		initProcess:        &processCacheEntry{exitCode: -1, isInitProcess: true},
	}
}
func (e *containerCacheEntry) AddNetworkAdapter(adapter prot.NetworkAdapter) {
	e.NetworkAdapters = append(e.NetworkAdapters, adapter)
}
func (e *containerCacheEntry) AddMappedVirtualDisk(disk prot.MappedVirtualDisk) error {
	if _, ok := e.MappedVirtualDisks[disk.Lun]; ok {
		return errors.Errorf("a mapped virtual disk with lun %d is already attached to container %s", disk.Lun, e.ID)
	}
	e.MappedVirtualDisks[disk.Lun] = disk
	return nil
}
func (e *containerCacheEntry) RemoveMappedVirtualDisk(disk prot.MappedVirtualDisk) {
	if _, ok := e.MappedVirtualDisks[disk.Lun]; !ok {
		logrus.Warnf("attempt to remove virtual disk with lun %d which is not attached to container %s", disk.Lun, e.ID)
		return
	}
	delete(e.MappedVirtualDisks, disk.Lun)
}
func (e *containerCacheEntry) AddMappedDirectory(dir prot.MappedDirectory) error {
	if _, ok := e.MappedDirectories[dir.Port]; ok {
		return errors.Errorf("a mapped directory with port %d is already attached to container %s", dir.Port, e.ID)
	}
	e.MappedDirectories[dir.Port] = dir
	return nil
}
func (e *containerCacheEntry) RemoveMappedDirectory(dir prot.MappedDirectory) {
	if _, ok := e.MappedDirectories[dir.Port]; !ok {
		logrus.Warnf("attempt to remove mapped directory with port %d which is not attached to container %s", dir.Port, e.ID)
		return
	}
	delete(e.MappedDirectories, dir.Port)
}

// processCacheEntry stores cached information for a single process.
type processCacheEntry struct {
	// Set to true only when this process is a container init process that is
	// associated with a container exited notification and needs to have the
	// writers tracked.
	isInitProcess bool
	Tty           *stdio.TtyRelay

	// Signaled when the process itself has exited.
	exitWg sync.WaitGroup
	// The exitCode set prior to signaling the exitWg
	exitCode int

	// Used to allow addtion/removal to the writersWg after an initial wait has
	// already been issued. It is not safe to call Add/Done without holding this
	// lock.
	writersSyncRoot sync.Mutex
	// Used to track the number of writers that need to finish
	// before the container can be marked as exited.
	writersWg sync.WaitGroup
	// Used to track the 1st caller to the writersWg that successfully
	// acknowledges it wrote the exit response.
	writersCalled bool
}

func (c *gcsCore) getContainer(id string) *containerCacheEntry {
	if entry, ok := c.containerCache[id]; ok {
		return entry
	}
	return nil
}

// CreateContainer creates all the infrastructure for a container, including
// setting up layers and networking, and then starts up its init process in a
// suspended state waiting for a call to StartContainer.
func (c *gcsCore) CreateContainer(id string, settings prot.VMHostedContainerSettings) error {
	c.containerCacheMutex.Lock()
	defer c.containerCacheMutex.Unlock()

	if c.getContainer(id) != nil {
		return gcserr.NewHresultError(gcserr.HrVmcomputeSystemAlreadyExists)
	}

	containerEntry := newContainerCacheEntry(id)
	// We need to only allow exited notifications when at least one WaitProcess
	// call has been written. We increment the writers here which is safe even
	// on failure because this entry will not be in the map on failure.
	logrus.Debugf("+1 initprocess.writersWg [gcsCore::CreateContainer]")
	containerEntry.initProcess.writersWg.Add(1)

	// Set up mapped virtual disks.
	if err := c.setupMappedVirtualDisks(id, settings.MappedVirtualDisks); err != nil {
		return errors.Wrapf(err, "failed to set up mapped virtual disks during create for container %s", id)
	}
	for _, disk := range settings.MappedVirtualDisks {
		containerEntry.AddMappedVirtualDisk(disk)
	}
	// Set up mapped directories.
	if err := c.setupMappedDirectories(id, settings.MappedDirectories); err != nil {
		return errors.Wrapf(err, "failed to set up mapped directories during create for container %s", id)
	}
	for _, dir := range settings.MappedDirectories {
		containerEntry.AddMappedDirectory(dir)
	}

	// Set up layers.
	scratch, layers, err := c.getLayerMounts(settings.SandboxDataPath, settings.Layers)
	if err != nil {
		return errors.Wrapf(err, "failed to get layer devices for container %s", id)
	}
	containerEntry.Index, err = c.getOrAddContainerIndex(id)
	if err != nil {
		return errors.Wrap(err, "failed to get a valid container index")
	}

	if err := c.mountLayers(containerEntry.Index, scratch, layers); err != nil {
		return errors.Wrapf(err, "failed to mount layers for container %s", id)
	}

	// Stash network adapters away
	for _, adapter := range settings.NetworkAdapters {
		containerEntry.AddNetworkAdapter(adapter)
	}
	// Create the directory that will contain the resolv.conf file.
	//
	// TODO(rn): This isn't quite right but works. Basically, when
	// we do the network config in ExecProcess() the overlay for
	// the rootfs has already been created. When we then write
	// /etc/resolv.conf to the base layer it won't show up unless
	// /etc exists when the overlay is created. This is a bit
	// problematic as we basically later write to a what is
	// supposed to be read-only layer in the overlay...  Ideally,
	// dockerd would pass a runc config with a bind mount for
	// /etc/resolv.conf like it does on unix.
	if err := os.MkdirAll(filepath.Join(baseFilesPath, "etc"), 0755); err != nil {
		return errors.Wrapf(err, "failed to create resolv.conf directory")
	}

	c.containerCache[id] = containerEntry

	return nil
}

// ExecProcess executes a new process in the container. It forwards the
// process's stdio through the members of the core.StdioSet provided.
func (c *gcsCore) ExecProcess(id string, params prot.ProcessParameters, connection stdio.ConnectionSettings) (_ int, _ chan<- struct{}, err error) {
	var stdioSet *stdio.ConnectionSet
	stdioSet, err = stdio.Connect(c.vsock, connection)
	if err != nil {
		return -1, nil, err
	}
	defer func() {
		if err != nil {
			stdioSet.Close()
		}
	}()

	c.containerCacheMutex.Lock()
	defer c.containerCacheMutex.Unlock()

	containerEntry := c.getContainer(id)
	if containerEntry == nil {
		return -1, nil, gcserr.NewHresultError(gcserr.HrVmcomputeSystemNotFound)
	}
	var processEntry *processCacheEntry
	if !containerEntry.hasRunInitProcess {
		processEntry = containerEntry.initProcess
	} else {
		processEntry = &processCacheEntry{exitCode: -1}
	}

	var pid int
	if !containerEntry.hasRunInitProcess {
		// Setup the error waiter
		execInitErrorDone := make(chan struct{})
		containerEntry.initProcess.writersSyncRoot.Lock()
		containerEntry.initProcess.writersWg.Add(1)
		containerEntry.initProcess.writersSyncRoot.Unlock()
		logrus.Debugf("+1 initprocess.writersWg [gcsCore::ExecProcess]")
		go func() {
			// Wait for the caller to notify they have handled the error.
			<-execInitErrorDone

			// Remove our waiter.
			logrus.Debugf("-1 initprocess.writersWg [gcsCore::ExecProcess goroutine]")
			containerEntry.initProcess.writersWg.Done()
			close(execInitErrorDone)
		}()
		containerEntry.hasRunInitProcess = true
		if err = c.writeConfigFile(containerEntry.Index, params.OCISpecification); err != nil {
			// Early exit. Cleanup our waiter since we never got a process.
			logrus.Debugf("-1 initprocess.writersWg [gcsCore::ExecProcess Error handling for writeConfigFile]")
			containerEntry.initProcess.writersWg.Done()
			return -1, execInitErrorDone, err
		}

		var container runtime.Container
		container, err = c.Rtime.CreateContainer(id, c.getContainerStoragePath(containerEntry.Index), stdioSet)
		if err != nil {
			// Early exit. Cleanup our waiter since we never got a process.
			logrus.Debugf("-1 initprocess.writersWg [gcsCore::ExecProcess Error handling for CreateContainerStoragePath]")
			containerEntry.initProcess.writersWg.Done()
			return -1, execInitErrorDone, err
		}

		containerEntry.container = container
		pid = container.Pid()
		containerEntry.initProcess.exitWg.Add(1)
		containerEntry.initProcess.Tty = container.Tty()

		// Configure network adapters in the namespace.
		for _, adapter := range containerEntry.NetworkAdapters {
			if err = c.configureAdapterInNamespace(container, adapter); err != nil {
				// Early exit. Cleanup our waiter since our init process is invalid.
				logrus.Debugf("-1 initprocess.writersWg [gcsCore::ExecProcess Error handling for configureAdapterInNamespace] %s", err)
				containerEntry.initProcess.writersWg.Done()
				return -1, execInitErrorDone, err
			}
		}

		go func() {
			// If we fail to cleanup the container we cannot reuse the storage location.
			leakContainerIndex := false
			exitCode, werr := container.Wait()
			c.containerCacheMutex.Lock()
			if werr != nil {
				logrus.Error(werr)
			}
			logrus.Debugf("gcsCore::ExecProcess container init process %d exited with exit status %d", container.Pid(), exitCode)

			if werr := c.cleanupContainer(containerEntry); werr != nil {
				logrus.Error(werr)
				leakContainerIndex = true
			}
			c.containerCacheMutex.Unlock()

			// We are the only writer. Safe to do without a lock
			containerEntry.initProcess.exitCode = exitCode
			containerEntry.initProcess.exitWg.Done()

			if !leakContainerIndex {
				c.removeContainerIndex(id)
			}

			c.containerCacheMutex.Lock()
			// This is safe because the init process WaitContainer has already
			// been initiated and thus removing from the map will not remove its
			// reference to the actual cacheEntry
			delete(c.containerCache, id)
			c.containerCacheMutex.Unlock()
		}()

		if err = container.Start(); err != nil {
			// Early exit. Cleanup our waiter since we never got a process.
			containerEntry.initProcess.writersWg.Done()
			return -1, execInitErrorDone, err
		}
	} else {
		var ociProcess *oci.Process
		ociProcess, err = processParametersToOCI(params)
		if err != nil {
			return -1, nil, err
		}
		var p runtime.Process
		p, err = containerEntry.container.ExecProcess(ociProcess, stdioSet)
		if err != nil {
			return -1, nil, err
		}
		pid = p.Pid()
		processEntry.exitWg.Add(1)
		processEntry.Tty = p.Tty()

		go func() {
			exitCode, werr := p.Wait()
			if werr != nil {
				logrus.Error(werr)
			}
			logrus.Infof("container process %d exited with exit status %d", p.Pid(), exitCode)

			processEntry.exitCode = exitCode
			processEntry.exitWg.Done()

			if derr := p.Delete(); derr != nil {
				logrus.Error(derr)
			}
		}()
	}

	c.processCacheMutex.Lock()
	// If a processCacheEntry with the given pid already exists in the cache,
	// this will overwrite it. This behavior is expected. Processes are kept in
	// the cache even after they exit, which allows for exit hooks registered
	// on exited processed to still run. For example, if the HCS were to wait
	// on a process which had already exited (due to a race condition between
	// the wait call and the process exiting), the process's exit state would
	// still be available to send back to the HCS. However, when pids are
	// reused on the system, it makes sense to overwrite the old cache entry.
	// This is because registering an exit hook on the pid and expecting it to
	// apply to the old process no longer makes sense, so since the old
	// process's pid has been reused, its cache entry can also be reused.  This
	// applies to external processes as well.
	c.processCache[pid] = processEntry
	c.processCacheMutex.Unlock()
	return pid, nil, nil
}

// SignalContainer sends the specified signal to the container's init process.
func (c *gcsCore) SignalContainer(id string, signal syscall.Signal) error {
	c.containerCacheMutex.Lock()
	defer c.containerCacheMutex.Unlock()

	containerEntry := c.getContainer(id)
	if containerEntry == nil {
		return gcserr.NewHresultError(gcserr.HrVmcomputeSystemNotFound)
	}

	if containerEntry.container != nil {
		if err := containerEntry.container.Kill(signal); err != nil {
			return err
		}
	}

	return nil
}

// SignalProcess sends the signal specified in options to the given process.
func (c *gcsCore) SignalProcess(pid int, options prot.SignalProcessOptions) error {
	c.processCacheMutex.Lock()
	if _, ok := c.processCache[pid]; !ok {
		c.processCacheMutex.Unlock()
		return gcserr.NewHresultError(gcserr.HrErrNotFound)
	}
	c.processCacheMutex.Unlock()

	// Interpret signal value 0 as SIGKILL.
	// TODO: Remove this special casing when we are not worried about breaking
	// older Windows builds which don't support sending signals.
	var signal syscall.Signal
	if options.Signal == 0 {
		signal = unix.SIGKILL
	} else {
		signal = syscall.Signal(options.Signal)
	}

	if err := syscall.Kill(pid, signal); err != nil {
		return errors.Wrapf(err, "failed call to kill on process %d with signal %d", pid, options.Signal)
	}

	return nil
}

// GetProperties returns the properties of the compute system.
func (c *gcsCore) GetProperties(id string, query string) (*prot.Properties, error) {
	c.containerCacheMutex.Lock()
	defer c.containerCacheMutex.Unlock()

	containerEntry := c.getContainer(id)
	if containerEntry == nil {
		return nil, gcserr.NewHresultError(gcserr.HrVmcomputeSystemNotFound)
	}
	if containerEntry.container == nil {
		return nil, nil
	}

	var queryObj prot.PropertyQuery
	if len(query) != 0 {
		if err := json.Unmarshal([]byte(query), &queryObj); err != nil {
			e := gcserr.WrapHresult(err, gcserr.HrVmcomputeInvalidJSON)
			return nil, errors.Wrapf(e, "The query could not be unmarshaled: '%s'", query)
		}
	}

	var properties prot.Properties
	for _, property := range queryObj.PropertyTypes {
		if property == prot.PtProcessList {
			processes, err := containerEntry.container.GetAllProcesses()
			if err != nil {
				return nil, err
			}
			processDetails := make([]prot.ProcessDetails, len(processes))
			for i, p := range processes {
				processDetails[i] = prot.ProcessDetails{ProcessID: uint32(p.Pid)}
			}
			properties.ProcessList = processDetails
		}
	}

	return &properties, nil
}

// RunExternalProcess runs a process in the utility VM outside of a container's
// namespace.
// This can be used for things like debugging or diagnosing the utility VM's
// state.
func (c *gcsCore) RunExternalProcess(params prot.ProcessParameters, conSettings stdio.ConnectionSettings) (_ int, err error) {
	var stdioSet *stdio.ConnectionSet
	stdioSet, err = stdio.Connect(c.vsock, conSettings)
	if err != nil {
		return -1, err
	}
	defer func() {
		if err != nil {
			stdioSet.Close()
		}
	}()

	var ociProcess *oci.Process
	ociProcess, err = processParametersToOCI(params)
	if err != nil {
		return -1, err
	}
	cmd := exec.Command(ociProcess.Args[0], ociProcess.Args[1:]...)
	cmd.Dir = ociProcess.Cwd
	cmd.Env = ociProcess.Env

	var relay *stdio.TtyRelay
	if params.EmulateConsole {
		// Allocate a console for the process.
		var (
			master      *os.File
			consolePath string
		)
		master, consolePath, err = stdio.NewConsole()
		if err != nil {
			return -1, errors.Wrap(err, "failed to create console for external process")
		}
		defer func() {
			if err != nil {
				master.Close()
			}
		}()

		var console *os.File
		console, err = os.OpenFile(consolePath, os.O_RDWR|syscall.O_NOCTTY, 0777)
		if err != nil {
			return -1, errors.Wrap(err, "failed to open console file for external process")
		}
		defer console.Close()

		relay = stdio.NewTtyRelay(stdioSet, master)
		cmd.Stdin = console
		cmd.Stdout = console
		cmd.Stderr = console
		// Make the child process a session leader and adopt the pty as
		// the controlling terminal.
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setsid:  true,
			Setctty: true,
			Ctty:    syscall.Stdin,
		}
	} else {
		var fileSet *stdio.FileSet
		fileSet, err = stdioSet.Files()
		if err != nil {
			return -1, errors.Wrap(err, "failed to set cmd stdio")
		}
		defer fileSet.Close()
		defer stdioSet.Close()
		cmd.Stdin = fileSet.In
		cmd.Stdout = fileSet.Out
		cmd.Stderr = fileSet.Err
	}
	if err = cmd.Start(); err != nil {
		return -1, errors.Wrap(err, "failed call to Start for external process")
	}

	if relay != nil {
		relay.Start()
	}

	processEntry := &processCacheEntry{exitCode: -1}
	processEntry.exitWg.Add(1)
	processEntry.Tty = relay
	go func() {
		if werr := cmd.Wait(); werr != nil {
			// TODO: When cmd is a shell, and last command in the shell
			// returned an error (e.g. typing a non-existing command gives
			// error 127), Wait also returns an error. We should find a way to
			// distinguish between these errors and ones which are actually
			// important.
			logrus.Error(errors.Wrap(werr, "failed call to Wait for external process"))
		}
		exitCode := cmd.ProcessState.ExitCode()
		logrus.Infof("external process %d exited with exit status %d", cmd.Process.Pid, exitCode)

		if relay != nil {
			relay.Wait()
		}

		// We are the only writer safe to do without a lock.
		processEntry.exitCode = exitCode
		processEntry.exitWg.Done()
	}()

	pid := cmd.Process.Pid
	c.processCacheMutex.Lock()
	c.processCache[pid] = processEntry
	c.processCacheMutex.Unlock()
	return pid, nil
}

// ModifySettings takes the given request and performs the modification it
// specifies. At the moment, this function only supports the request types Add
// and Remove, both for the resource type MappedVirtualDisk.
func (c *gcsCore) ModifySettings(id string, request *prot.ResourceModificationRequestResponse) error {
	c.containerCacheMutex.Lock()
	defer c.containerCacheMutex.Unlock()

	containerEntry := c.getContainer(id)
	if containerEntry == nil {
		return gcserr.NewHresultError(gcserr.HrVmcomputeSystemNotFound)
	}

	switch request.ResourceType {
	case prot.PtMappedVirtualDisk:
		mvd, ok := request.Settings.(*prot.MappedVirtualDisk)
		if !ok {
			return errors.New("the request's settings are not of type MappedVirtualDisk")
		}
		switch request.RequestType {
		case prot.RtAdd:
			if err := c.setupMappedVirtualDisks(id, []prot.MappedVirtualDisk{*mvd}); err != nil {
				return errors.Wrapf(err, "failed to hot add mapped virtual disk for container %s", id)
			}
			containerEntry.AddMappedVirtualDisk(*mvd)
		case prot.RtRemove:
			// If the disk was specified AttachOnly, it shouldn't have been mounted
			// in the first place.
			if !mvd.AttachOnly {
				if err := storage.UnmountPath(mvd.ContainerPath, false); err != nil {
					return errors.Wrapf(err, "failed to unmount mapped virtual disks for container %s", id)
				}
			}
			if err := scsi.UnplugDevice(0, mvd.Lun); err != nil {
				return errors.Wrapf(err, "failed to unplug mapped virtual disks for container %s, scsi lun: %d", id, mvd.Lun)
			}
			containerEntry.RemoveMappedVirtualDisk(*mvd)
		default:
			return errors.Errorf("the request type \"%s\" is not supported for resource type \"%s\"", request.RequestType, request.ResourceType)
		}
	case prot.PtMappedDirectory:
		md, ok := request.Settings.(*prot.MappedDirectory)
		if !ok {
			return errors.New("the request's settings are not of type MappedDirectory")
		}
		switch request.RequestType {
		case prot.RtAdd:
			if err := c.setupMappedDirectories(id, []prot.MappedDirectory{*md}); err != nil {
				return errors.Wrapf(err, "failed to hot add mapped directory for container %s", id)
			}
			containerEntry.AddMappedDirectory(*md)
		case prot.RtRemove:
			if err := storage.UnmountPath(md.ContainerPath, false); err != nil {
				return errors.Wrapf(err, "failed to mount mapped directories for container %s", id)
			}
			containerEntry.RemoveMappedDirectory(*md)
		default:
			return errors.Errorf("the request type \"%s\" is not supported for resource type \"%s\"", request.RequestType, request.ResourceType)
		}
	default:
		return errors.Errorf("the resource type \"%s\" is not supported", request.ResourceType)
	}

	return nil
}

func (c *gcsCore) ResizeConsole(pid int, height, width uint16) error {
	c.processCacheMutex.Lock()
	var p *processCacheEntry
	var ok bool
	if p, ok = c.processCache[pid]; !ok {
		c.processCacheMutex.Unlock()
		return gcserr.NewHresultError(gcserr.HrErrNotFound)
	}
	c.processCacheMutex.Unlock()

	if p.Tty == nil {
		return fmt.Errorf("pid: %d, is not a tty and cannot be resized", pid)
	}

	return p.Tty.ResizeConsole(height, width)
}

// WaitContainer returns a function that can be used to sucessfully wait for a
// container. This will only return after all writers on WaitProcess have
// completed. On error the container id was not a valid container.
func (c *gcsCore) WaitContainer(id string) (func() prot.NotificationType, error) {
	c.containerCacheMutex.Lock()
	entry := c.getContainer(id)
	if entry == nil {
		c.containerCacheMutex.Unlock()
		return nil, gcserr.NewHresultError(gcserr.HrVmcomputeSystemNotFound)
	}
	c.containerCacheMutex.Unlock()

	f := func() prot.NotificationType {
		logrus.Debugf("gcscore::WaitContainer waiting on init process waitgroup")
		entry.initProcess.writersWg.Wait()
		logrus.Debugf("gcscore::WaitContainer init process waitgroup count has dropped to zero")
		// v1 only supported unexpected exit
		return prot.NtUnexpectedExit
	}

	return f, nil
}

// WaitProcess returns a channel that can be used to wait for the process exit
// code and a channel that can be used to signal when the waiter has processed
// the code fully or decided to stop waiting.
//
// The second channel must be signaled in either case to keep the wait counts in
// sync.
//
// On error the pid was not a valid pid and no channels will be returned.
func (c *gcsCore) WaitProcess(pid int) (<-chan int, chan<- bool, error) {
	c.processCacheMutex.Lock()
	entry, ok := c.processCache[pid]
	if !ok {
		c.processCacheMutex.Unlock()
		return nil, nil, gcserr.NewHresultError(gcserr.HrErrNotFound)
	}
	c.processCacheMutex.Unlock()

	// If we are an init process waiter increment our count for this waiter.
	if entry.isInitProcess {
		entry.writersSyncRoot.Lock()
		logrus.Debugf("gcscore::WaitProcess Incrementing waitgroup as isInitProcess")
		entry.writersWg.Add(1)
		entry.writersSyncRoot.Unlock()
	}

	exitCodeChan := make(chan int, 1)
	doneChan := make(chan bool)

	go func() {
		bgExitCodeChan := make(chan int, 1)
		go func() {
			entry.exitWg.Wait()
			bgExitCodeChan <- entry.exitCode
		}()

		// Wait for the exit code or the caller to stop waiting.
		select {
		case exitCode := <-bgExitCodeChan:
			logrus.Debugf("gcscore::WaitProcess got an exitCode %d", exitCode)
			// We got an exit code tell our caller.
			exitCodeChan <- exitCode

			// Wait for the caller to tell us they have issued the write and
			// release the writers count.
			select {
			case <-doneChan:
				if entry.isInitProcess {
					entry.writersSyncRoot.Lock()
					// Decrement this waiter
					logrus.Debugf("-1 writersWg [gcsCore::WaitProcess] exitCode from bgExitCodeChan doneChan")
					entry.writersWg.Done()
					if !entry.writersCalled {
						// Decrement the container exited waiter now that we
						// know we have successfully written at least 1
						// WaitProcess on the init process.
						logrus.Debugf("-1 writersWg [gcsCore::WaitProcess] exitCode from bgExitCodeChan, !writersCalled")
						entry.writersCalled = true
						entry.writersWg.Done()
					}
					entry.writersSyncRoot.Unlock()
				}
			}
		case <-doneChan:
			logrus.Debugf("gcscore::WaitProcess done channel")
			// This case means that the waiter decided to stop waiting before
			// the process had an exit code. In this case we need to cleanup
			// just our waiter because the no response was written.
			if entry.isInitProcess {
				logrus.Debugf("-1 writersWg [gcsCore::WaitProcess] doneChan")
				entry.writersSyncRoot.Lock()
				entry.writersWg.Done()
				entry.writersSyncRoot.Unlock()
			}
		}
	}()

	return exitCodeChan, doneChan, nil
}

// setupMappedVirtualDisks is a helper function which calls into the functions
// in storage.go to set up a set of mapped virtual disks for a given container.
// It then adds them to the container's cache entry.
// This function expects containerCacheMutex to be locked on entry.
func (c *gcsCore) setupMappedVirtualDisks(id string, disks []prot.MappedVirtualDisk) error {
	mounts, err := c.getMappedVirtualDiskMounts(disks)
	if err != nil {
		return errors.Wrapf(err, "failed to get mapped virtual disk devices for container %s", id)
	}
	if err := c.mountMappedVirtualDisks(disks, mounts); err != nil {
		return errors.Wrapf(err, "failed to mount mapped virtual disks for container %s", id)
	}
	return nil
}

// setupMappedDirectories is a helper function which calls into the functions
// in storage.go to set up a set of mapped directories for a given container.
// It then adds them to the container's cache entry.
// This function expects containerCacheMutex to be locked on entry.
func (c *gcsCore) setupMappedDirectories(id string, dirs []prot.MappedDirectory) error {
	for _, dir := range dirs {
		if !dir.CreateInUtilityVM {
			return errors.New("we do not currently support mapping directories inside the container namespace")
		}
		if err := plan9.Mount(c.vsock, dir.ContainerPath, "", dir.Port, dir.ReadOnly); err != nil {
			return errors.Wrapf(err, "failed to mount mapped directory %s for container %s", dir.ContainerPath, id)
		}
	}
	return nil
}

// processParametersToOCI converts the given ProcessParameters struct into an
// oci.Process struct for OCI version 1.0.0. Since ProcessParameters
// doesn't include various fields which are available in oci.Process, default
// values for these fields are chosen.
func processParametersToOCI(params prot.ProcessParameters) (*oci.Process, error) {
	if params.OCIProcess != nil {
		return params.OCIProcess, nil
	}

	var args []string
	if len(params.CommandArgs) == 0 {
		var err error
		args, err = processParamCommandLineToOCIArgs(params.CommandLine)
		if err != nil {
			return new(oci.Process), err
		}
	} else {
		args = params.CommandArgs
	}
	return &oci.Process{
		Args:     args,
		Cwd:      params.WorkingDirectory,
		Env:      processParamEnvToOCIEnv(params.Environment),
		Terminal: params.EmulateConsole,

		// TODO: We might want to eventually choose alternate default values
		// for these.
		User: oci.User{UID: 0, GID: 0},
		Capabilities: &oci.LinuxCapabilities{
			Bounding: []string{
				"CAP_AUDIT_WRITE",
				"CAP_KILL",
				"CAP_NET_BIND_SERVICE",
				"CAP_SYS_ADMIN",
				"CAP_NET_ADMIN",
				"CAP_SETGID",
				"CAP_SETUID",
				"CAP_CHOWN",
				"CAP_FOWNER",
				"CAP_DAC_OVERRIDE",
				"CAP_NET_RAW",
			},
			Effective: []string{
				"CAP_AUDIT_WRITE",
				"CAP_KILL",
				"CAP_NET_BIND_SERVICE",
				"CAP_SYS_ADMIN",
				"CAP_NET_ADMIN",
				"CAP_SETGID",
				"CAP_SETUID",
				"CAP_CHOWN",
				"CAP_FOWNER",
				"CAP_DAC_OVERRIDE",
				"CAP_NET_RAW",
			},
			Inheritable: []string{
				"CAP_AUDIT_WRITE",
				"CAP_KILL",
				"CAP_NET_BIND_SERVICE",
				"CAP_SYS_ADMIN",
				"CAP_NET_ADMIN",
				"CAP_SETGID",
				"CAP_SETUID",
				"CAP_CHOWN",
				"CAP_FOWNER",
				"CAP_DAC_OVERRIDE",
				"CAP_NET_RAW",
			},
			Permitted: []string{
				"CAP_AUDIT_WRITE",
				"CAP_KILL",
				"CAP_NET_BIND_SERVICE",
				"CAP_SYS_ADMIN",
				"CAP_NET_ADMIN",
				"CAP_SETGID",
				"CAP_SETUID",
				"CAP_CHOWN",
				"CAP_FOWNER",
				"CAP_DAC_OVERRIDE",
				"CAP_NET_RAW",
			},
			Ambient: []string{
				"CAP_AUDIT_WRITE",
				"CAP_KILL",
				"CAP_NET_BIND_SERVICE",
				"CAP_SYS_ADMIN",
				"CAP_NET_ADMIN",
				"CAP_SETGID",
				"CAP_SETUID",
				"CAP_CHOWN",
				"CAP_FOWNER",
				"CAP_DAC_OVERRIDE",
				"CAP_NET_RAW",
			},
		},
		Rlimits: []oci.POSIXRlimit{
			oci.POSIXRlimit{Type: "RLIMIT_NOFILE", Hard: 1024, Soft: 1024},
		},
		NoNewPrivileges: true,
	}, nil
}

// processParamCommandLineToOCIArgs converts a CommandLine field from
// ProcessParameters (a space separate argument string) into an array of string
// arguments which can be used by an oci.Process.
func processParamCommandLineToOCIArgs(commandLine string) ([]string, error) {
	args, err := shellwords.Parse(commandLine)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse command line string \"%s\"", commandLine)
	}
	return args, nil
}

// processParamEnvToOCIEnv converts an Environment field from ProcessParameters
// (a map from environment variable to value) into an array of environment
// variable assignments (where each is in the form "<variable>=<value>") which
// can be used by an oci.Process.
func processParamEnvToOCIEnv(environment map[string]string) []string {
	environmentList := make([]string, 0, len(environment))
	for k, v := range environment {
		// TODO: Do we need to escape things like quotation marks in
		// environment variable values?
		environmentList = append(environmentList, fmt.Sprintf("%s=%s", k, v))
	}
	return environmentList
}

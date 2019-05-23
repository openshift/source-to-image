package runc

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	"github.com/pkg/errors"
)

// readPidFile reads the integer pid stored in the given file.
func (r *runcRuntime) readPidFile(pidFile string) (pid int, err error) {
	data, err := ioutil.ReadFile(pidFile)
	if err != nil {
		return -1, errors.Wrap(err, "failed reading from pid file")
	}
	pid, err = strconv.Atoi(string(data))
	if err != nil {
		return -1, errors.Wrapf(err, "failed converting pid text \"%s\" to integer form", data)
	}
	return pid, nil
}

// cleanupContainer cleans up any state left behind by the container.
func (r *runcRuntime) cleanupContainer(id string) error {
	containerDir := r.getContainerDir(id)
	if err := os.RemoveAll(containerDir); err != nil {
		return errors.Wrapf(err, "failed removing the container directory for container %s", id)
	}
	return nil
}

// cleanupProcess cleans up any state left behind by the process.
func (r *runcRuntime) cleanupProcess(id string, pid int) error {
	processDir := r.getProcessDir(id, pid)
	if err := os.RemoveAll(processDir); err != nil {
		return errors.Wrapf(err, "failed removing the process directory for process %d in container %s", pid, id)
	}
	return nil
}

// getProcessDir returns the path to the state directory of the given process.
func (r *runcRuntime) getProcessDir(id string, pid int) string {
	containerDir := r.getContainerDir(id)
	return filepath.Join(containerDir, strconv.Itoa(pid))
}

// getContainerDir returns the path to the state directory of the given
// container.
func (r *runcRuntime) getContainerDir(id string) string {
	return filepath.Join(containerFilesDir, id)
}

// makeContainerDir creates the state directory for the given container.
func (r *runcRuntime) makeContainerDir(id string) error {
	dir := r.getContainerDir(id)
	if err := os.MkdirAll(dir, os.ModeDir); err != nil {
		return errors.Wrapf(err, "failed making container directory for container %s", id)
	}
	return nil
}

// getLogDir gets the path to the runc logs directory.
func (r *runcRuntime) getLogDir(id string) string {
	return filepath.Join(r.runcLogBasePath, id)
}

// makeLogDir creates the runc logs directory if it doesnt exist.
func (r *runcRuntime) makeLogDir(id string) error {
	dir := r.getLogDir(id)
	if err := os.MkdirAll(dir, os.ModeDir); err != nil {
		return errors.Wrapf(err, "failed making runc log directory for container %s", id)
	}
	return nil
}

// getLogPath returns the path to the log file used by the runC wrapper.
func (r *runcRuntime) getLogPath(id string) string {
	return filepath.Join(r.getLogDir(id), "runc.log")
}

// processExists returns true if the given process exists in /proc, false if
// not.
// It should be noted that processes which have exited, but have not yet been
// waited on (i.e. zombies) are still considered to exist by this function.
func (r *runcRuntime) processExists(pid int) bool {
	_, err := os.Stat(filepath.Join("/proc", strconv.Itoa(pid)))
	return !os.IsNotExist(err)
}

package runc

import (
	"net"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

// createConsoleSocket creates a unix socket in the given process directory and
// returns its path and a listener to it. This socket can then be used to
// receive the container's terminal master file descriptor.
func (r *runcRuntime) createConsoleSocket(processDir string) (listener *net.UnixListener, socketPath string, err error) {
	socketPath = filepath.Join(processDir, "master.sock")
	addr, err := net.ResolveUnixAddr("unix", socketPath)
	if err != nil {
		return nil, "", errors.Wrapf(err, "failed to resolve unix socket at address %s", socketPath)
	}
	listener, err = net.ListenUnix("unix", addr)
	if err != nil {
		return nil, "", errors.Wrapf(err, "failed to listen on unix socket at address %s", socketPath)
	}
	return listener, socketPath, nil
}

// getMasterFromSocket blocks on the given listener's socket until a message is
// sent, then parses the file descriptor representing the terminal master out
// of the message and returns it as a file.
func (r *runcRuntime) getMasterFromSocket(listener *net.UnixListener) (master *os.File, err error) {
	// Accept the listener's connection.
	conn, err := listener.Accept()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get terminal master file descriptor from socket")
	}
	defer conn.Close()
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return nil, errors.New("connection returned from Accept was not a unix socket")
	}

	const maxNameLen = 4096
	var oobSpace = unix.CmsgSpace(4)

	name := make([]byte, maxNameLen)
	oob := make([]byte, oobSpace)

	// Read a message from the unix socket. This blocks until the message is
	// sent.
	n, oobn, _, _, err := unixConn.ReadMsgUnix(name, oob)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read message from unix socket")
	}
	if n >= maxNameLen || oobn != oobSpace {
		return nil, errors.Errorf("read an invalid number of bytes (n=%d oobn=%d)", n, oobn)
	}

	// Truncate the data returned from the message.
	name = name[:n]
	oob = oob[:oobn]

	// Parse the out-of-band data in the message.
	messages, err := unix.ParseSocketControlMessage(oob)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse socket control message for oob %v", oob)
	}
	if len(messages) == 0 {
		return nil, errors.New("did not receive any socket control messages")
	}
	if len(messages) > 1 {
		return nil, errors.Errorf("received more than one socket control message: received %d", len(messages))
	}
	message := messages[0]

	// Parse the file descriptor out of the out-of-band data in the message.
	fds, err := unix.ParseUnixRights(&message)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse file descriptors out of message %v", message)
	}
	if len(fds) == 0 {
		return nil, errors.New("did not receive any file descriptors")
	}
	if len(fds) > 1 {
		return nil, errors.Errorf("received more than one file descriptor: received %d", len(fds))
	}
	fd := uintptr(fds[0])

	return os.NewFile(fd, string(name)), nil
}

// pathExists returns true if the given path exists, false if not.
func (r *runcRuntime) pathExists(pathToCheck string) (bool, error) {
	_, err := os.Stat(pathToCheck)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, errors.Wrapf(err, "failed call to Stat for path %s", pathToCheck)
	}
	return true, nil
}

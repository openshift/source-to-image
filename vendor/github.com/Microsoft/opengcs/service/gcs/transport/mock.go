package transport

import (
	"net"
	"os"
	"syscall"

	"github.com/pkg/errors"
)

// MockTransport is a mock implementation of Transport.
type MockTransport struct {
	// Channel sends connections to the "server" once the "client" has
	// connected.
	Channel chan *MockConnection
}

// Dial ignores the port, and returns a MockTransport struct.
func (t *MockTransport) Dial(_ uint32) (_ Connection, err error) {
	var fds [2]int
	var serverFile *os.File
	var clientFile *os.File
	var serverConn net.Conn
	var clientConn net.Conn
	defer func() {
		if err != nil {
			syscall.Close(fds[0])
			syscall.Close(fds[1])
			serverFile.Close()
			clientFile.Close()
			serverConn.Close()
			clientConn.Close()
		}
	}()

	fds, err = syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, errors.Wrap(err, "failed call to Socketpair for unix sockets")
	}
	serverFile = os.NewFile(uintptr(fds[0]), "")
	clientFile = os.NewFile(uintptr(fds[1]), "")
	serverConn, err = net.FileConn(serverFile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create mock server connection")
	}
	clientConn, err = net.FileConn(clientFile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create mock client connection")
	}
	serverUnixConn, ok := serverConn.(*net.UnixConn)
	if !ok {
		return nil, errors.New("server connection was not a unix socket")
	}
	clientUnixConn, ok := clientConn.(*net.UnixConn)
	if !ok {
		return nil, errors.New("client connection was not a unix socket")
	}

	if t.Channel != nil {
		t.Channel <- &MockConnection{
			UnixConn: serverUnixConn,
		}
	} else {
		serverUnixConn.Close()
	}

	return &MockConnection{
		UnixConn: clientUnixConn,
	}, nil
}

// MockConnection is a mock implementation of Connection.
type MockConnection struct {
	*net.UnixConn
}

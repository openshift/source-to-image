package stdio

import (
	"os"

	"github.com/Microsoft/opengcs/service/gcs/transport"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ConnectionSettings describe the stdin, stdout, stderr ports to connect the
// transport to. A nil port specifies no connection.
type ConnectionSettings struct {
	StdIn  *uint32
	StdOut *uint32
	StdErr *uint32
}

type logConnection struct {
	con  transport.Connection
	port uint32
}

func (lc *logConnection) Read(b []byte) (int, error) {
	return lc.con.Read(b)
}

func (lc *logConnection) Write(b []byte) (int, error) {
	return lc.con.Write(b)
}

func (lc *logConnection) Close() error {
	logrus.WithFields(logrus.Fields{
		"port": lc.port,
	}).Debug("opengcs::logConnection::Close - closing connection")

	return lc.con.Close()
}

func (lc *logConnection) CloseRead() error {
	logrus.WithFields(logrus.Fields{
		"port": lc.port,
	}).Debug("opengcs::logConnection::Close - closing read connection")

	return lc.con.CloseRead()
}

func (lc *logConnection) CloseWrite() error {
	logrus.WithFields(logrus.Fields{
		"port": lc.port,
	}).Debug("opengcs::logConnection::Close - closing write connection")

	return lc.con.CloseWrite()
}

func (lc *logConnection) File() (*os.File, error) {
	return lc.con.File()
}

var _ = (transport.Connection)(&logConnection{})

// Connect returns new transport.Connection instances, one for each stdio pipe
// to be used. If CreateStd*Pipe for a given pipe is false, the given Connection
// is set to nil.
func Connect(tport transport.Transport, settings ConnectionSettings) (_ *ConnectionSet, err error) {
	connSet := &ConnectionSet{}
	defer func() {
		if err != nil {
			connSet.Close()
		}
	}()
	if settings.StdIn != nil {
		c, err := tport.Dial(*settings.StdIn)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating stdin Connection")
		}
		connSet.In = &logConnection{
			con:  c,
			port: *settings.StdIn,
		}
	}
	if settings.StdOut != nil {
		c, err := tport.Dial(*settings.StdOut)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating stdout Connection")
		}
		connSet.Out = &logConnection{
			con:  c,
			port: *settings.StdOut,
		}
	}
	if settings.StdErr != nil {
		c, err := tport.Dial(*settings.StdErr)
		if err != nil {
			return nil, errors.Wrap(err, "failed creating stderr Connection")
		}
		connSet.Err = &logConnection{
			con:  c,
			port: *settings.StdErr,
		}
	}
	return connSet, nil
}

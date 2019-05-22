package stdio

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

// NewConsole allocates a new console and returns the File for its master and
// path for its slave.
func NewConsole() (*os.File, string, error) {
	master, err := os.OpenFile("/dev/ptmx", syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, "", errors.Wrap(err, "failed to open master pseudoterminal file")
	}
	console, err := ptsname(master)
	if err != nil {
		return nil, "", err
	}
	if err := unlockpt(master); err != nil {
		return nil, "", err
	}
	// TODO: Do we need to keep this chmod call?
	if err := os.Chmod(console, 0600); err != nil {
		return nil, "", errors.Wrap(err, "failed to change permissions on the slave pseudoterminal file")
	}
	if err := os.Chown(console, 0, 0); err != nil {
		return nil, "", errors.Wrap(err, "failed to change ownership on the slave pseudoterminal file")
	}
	return master, console, nil
}

// ResizeConsole sends the appropriate resize to a pTTY FD
// Synchronization of pty should be handled in the callers context.
func ResizeConsole(pty *os.File, height, width uint16) error {
	type consoleSize struct {
		Height uint16
		Width  uint16
		x      uint16
		y      uint16
	}

	return ioctl(pty.Fd(), uintptr(unix.TIOCSWINSZ), uintptr(unsafe.Pointer(&consoleSize{Height: height, Width: width})))
}

func ioctl(fd uintptr, flag, data uintptr) error {
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, flag, data); err != 0 {
		return err
	}
	return nil
}

// ptsname is a Go wrapper around the ptsname system call. It returns the name
// of the slave pseudoterminal device corresponding to the given master.
func ptsname(f *os.File) (string, error) {
	var n int32
	if err := ioctl(f.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&n))); err != nil {
		return "", errors.Wrap(err, "ioctl TIOCGPTN failed for ptsname")
	}
	return fmt.Sprintf("/dev/pts/%d", n), nil
}

// unlockpt is a Go wrapper around the unlockpt system call. It unlocks the
// slave pseudoterminal device corresponding to the given master.
func unlockpt(f *os.File) error {
	var u int32
	if err := ioctl(f.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&u))); err != nil {
		return errors.Wrap(err, "ioctl TIOCSPTLCK failed for unlockpt")
	}
	return nil
}

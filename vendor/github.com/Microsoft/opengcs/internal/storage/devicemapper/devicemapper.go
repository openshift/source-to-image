// +build linux

package devicemapper

import (
	"fmt"
	"os"
	"path"
	"unsafe"

	"golang.org/x/sys/unix"
)

// CreateFlags modify the operation of CreateDevice
type CreateFlags int

const (
	// CreateReadOnly specifies that the device is not writable
	CreateReadOnly CreateFlags = 1 << iota
)

const (
	_IOC_WRITE    = 1
	_IOC_READ     = 2
	_IOC_NRBITS   = 8
	_IOC_TYPEBITS = 8
	_IOC_SIZEBITS = 14
	_IOC_DIRBITS  = 2

	_IOC_NRMASK    = ((1 << _IOC_NRBITS) - 1)
	_IOC_TYPEMASK  = ((1 << _IOC_TYPEBITS) - 1)
	_IOC_SIZEMASK  = ((1 << _IOC_SIZEBITS) - 1)
	_IOC_DIRMASK   = ((1 << _IOC_DIRBITS) - 1)
	_IOC_TYPESHIFT = (_IOC_NRBITS)
	_IOC_SIZESHIFT = (_IOC_TYPESHIFT + _IOC_TYPEBITS)
	_IOC_DIRSHIFT  = (_IOC_SIZESHIFT + _IOC_SIZEBITS)

	_DM_IOCTL      = 0xfd
	_DM_IOCTL_SIZE = 312
	_DM_IOCTL_BASE = (_IOC_READ|_IOC_WRITE)<<_IOC_DIRSHIFT | _DM_IOCTL<<_IOC_TYPESHIFT | _DM_IOCTL_SIZE<<_IOC_SIZESHIFT

	_DM_READONLY_FLAG       = 1 << 0
	_DM_SUSPEND_FLAG        = 1 << 1
	_DM_PERSISTENT_DEV_FLAG = 1 << 3
)

const (
	_DM_VERSION = iota
	_DM_REMOVE_ALL
	_DM_LIST_DEVICES
	_DM_DEV_CREATE
	_DM_DEV_REMOVE
	_DM_DEV_RENAME
	_DM_DEV_SUSPEND
	_DM_DEV_STATUS
	_DM_DEV_WAIT
	_DM_TABLE_LOAD
	_DM_TABLE_CLEAR
	_DM_TABLE_DEPS
	_DM_TABLE_STATUS
)

var dmOpName = []string{
	"version",
	"remove all",
	"list devices",
	"device create",
	"device remove",
	"device rename",
	"device suspend",
	"device status",
	"device wait",
	"table load",
	"table clear",
	"table deps",
	"table status",
}

type dmIoctl struct {
	Version     [3]uint32
	DataSize    uint32
	DataStart   uint32
	TargetCount uint32
	OpenCount   int32
	Flags       uint32
	EventNumber uint32
	_           uint32
	Dev         uint64
	Name        [128]byte
	UUID        [129]byte
	_           [7]byte
}

type targetSpec struct {
	SectorStart int64
	Length      int64
	Status      int32
	Next        uint32
	Type        [16]byte
}

// initIoctl initializes a device-mapper ioctl input struct with the given size
// and device name
func initIoctl(d *dmIoctl, size int, name string) {
	*d = dmIoctl{
		Version:  [3]uint32{4, 0, 0},
		DataSize: uint32(size),
	}
	copy(d.Name[:], name)
}

type dmError struct {
	Op  int
	Err error
}

func (err *dmError) Error() string {
	op := "<bad operation>"
	if err.Op < len(dmOpName) {
		op = dmOpName[err.Op]
	}
	return "device-mapper " + op + ": " + err.Err.Error()
}

// ioctl issues the specified device-mapper ioctl
func ioctl(f *os.File, code int, data *dmIoctl) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), uintptr(code|_DM_IOCTL_BASE), uintptr(unsafe.Pointer(data)))
	if errno != 0 {
		return &dmError{Op: code, Err: errno}
	}
	return nil
}

// openMapper opens the device-mapper control device and validates that it
// supports the required version
func openMapper() (f *os.File, err error) {
	f, err = os.OpenFile("/dev/mapper/control", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()
	var d dmIoctl
	initIoctl(&d, int(unsafe.Sizeof(d)), "")
	err = ioctl(f, _DM_VERSION, &d)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// Target specifies a single entry in a device's target specification.
type Target struct {
	Type                string
	SectorStart, Length int64
	Params              string
}

// sizeof returns the size of a targetSpec needed to fit this specification.
func (t *Target) sizeof() int {
	// include a null terminator (not sure if necessary) and round up to 8-byte
	// alignment
	return (int(unsafe.Sizeof(targetSpec{})) + len(t.Params) + 1 + 7) &^ 7
}

// LinearTarget constructs a device-mapper target that maps a portion of a block
// device at the specified offset.
func LinearTarget(sectorStart, length int64, path string, deviceStart int64) Target {
	return Target{
		Type:        "linear",
		SectorStart: sectorStart,
		Length:      length,
		Params:      fmt.Sprintf("%s %d", path, deviceStart),
	}
}

// makeTableIoctl builds an ioctl input structure with a table of the speicifed
// targets.
func makeTableIoctl(name string, targets []Target) *dmIoctl {
	off := int(unsafe.Sizeof(dmIoctl{}))
	n := off
	for _, t := range targets {
		n += t.sizeof()
	}
	b := make([]byte, n)
	d := (*dmIoctl)(unsafe.Pointer(&b[0]))
	initIoctl(d, n, name)
	d.DataStart = uint32(off)
	d.TargetCount = uint32(len(targets))
	for _, t := range targets {
		spec := (*targetSpec)(unsafe.Pointer(&b[off]))
		sn := t.sizeof()
		spec.SectorStart = t.SectorStart
		spec.Length = t.Length
		spec.Next = uint32(sn)
		copy(spec.Type[:], t.Type)
		copy(b[off+int(unsafe.Sizeof(*spec)):], t.Params)
		off += int(sn)
	}
	return d
}

// CreateDevice creates a device-mapper device with the given target spec. It returns
// the path of the new device node.
func CreateDevice(name string, flags CreateFlags, targets []Target) (_ string, err error) {
	f, err := openMapper()
	if err != nil {
		return "", err
	}
	defer f.Close()

	var d dmIoctl
	size := int(unsafe.Sizeof(d))
	initIoctl(&d, size, name)
	err = ioctl(f, _DM_DEV_CREATE, &d)
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			removeDevice(f, name)
		}
	}()

	dev := int(d.Dev)

	di := makeTableIoctl(name, targets)
	if flags&CreateReadOnly != 0 {
		di.Flags |= _DM_READONLY_FLAG
	}
	err = ioctl(f, _DM_TABLE_LOAD, di)
	if err != nil {
		return "", err
	}
	initIoctl(&d, size, name)
	err = ioctl(f, _DM_DEV_SUSPEND, &d)
	if err != nil {
		return "", err
	}

	p := path.Join("/dev/mapper", name)
	os.Remove(p)
	err = unix.Mknod(p, unix.S_IFBLK|0600, int(dev))
	if err != nil {
		return "", nil
	}

	return p, nil
}

// RemoveDevice removes a device-mapper device and its associated device node.
func RemoveDevice(name string) error {
	f, err := openMapper()
	if err != nil {
		return err
	}
	defer f.Close()
	os.Remove(path.Join("/dev/mapper", name))
	return removeDevice(f, name)
}

func removeDevice(f *os.File, name string) error {
	var d dmIoctl
	initIoctl(&d, int(unsafe.Sizeof(d)), name)
	err := ioctl(f, _DM_DEV_REMOVE, &d)
	if err != nil {
		return err
	}
	return nil
}

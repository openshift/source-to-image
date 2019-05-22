// +build linux

package devicemapper

import (
	"flag"
	"os"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

var (
	integration = flag.Bool("integration", false, "run integration tests")
)

func TestMain(m *testing.M) {
	flag.Parse()
	m.Run()
}

func validateDevice(t *testing.T, p string, sectors int64, writable bool) {
	dev, err := os.OpenFile(p, os.O_RDWR|os.O_SYNC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer dev.Close()

	var size int64
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, dev.Fd(), unix.BLKGETSIZE64, uintptr(unsafe.Pointer(&size)))
	if errno != 0 {
		t.Fatal(errno)
	}
	if size != sectors*512 {
		t.Fatalf("expected %d bytes, got %d", sectors*512, size)
	}

	var b [512]byte
	_, err = unix.Read(int(dev.Fd()), b[:])
	if err != unix.EIO {
		t.Fatalf("expected EIO, got %s", err)
	}
	_, err = unix.Write(int(dev.Fd()), b[:])
	if writable {
		if err != unix.EIO {
			t.Fatalf("expected EIO, got %s", err)
		}
	} else if err != unix.EPERM {
		t.Fatalf("expected EPERM, got %s", err)
	}

}

type device struct {
	Name, Path string
}

func (d *device) Close() (err error) {
	if d.Name != "" {
		err = RemoveDevice(d.Name)
		if err == nil {
			d.Name = ""
		}
	}
	return
}

func createDevice(name string, flags CreateFlags, targets []Target) (*device, error) {
	p, err := CreateDevice(name, flags, targets)
	if err != nil {
		return nil, err
	}
	return &device{Name: name, Path: p}, nil
}

func TestCreateError(t *testing.T) {
	if !*integration {
		t.Skip()
	}
	d, err := createDevice("test-device", 0, []Target{
		{Type: "error", SectorStart: 0, Length: 1},
		{Type: "error", SectorStart: 1, Length: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	validateDevice(t, d.Path, 3, true)
	err = d.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestReadOnlyError(t *testing.T) {
	if !*integration {
		t.Skip()
	}
	d, err := createDevice("test-device", CreateReadOnly, []Target{
		{Type: "error", SectorStart: 0, Length: 1},
		{Type: "error", SectorStart: 1, Length: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	validateDevice(t, d.Path, 3, false)
	err = d.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func TestLinearError(t *testing.T) {
	if !*integration {
		t.Skip()
	}
	b, err := createDevice("base-device", 0, []Target{
		{Type: "error", SectorStart: 0, Length: 100},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	d, err := createDevice("linear-device", 0, []Target{
		LinearTarget(0, 50, b.Path, 50),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	validateDevice(t, d.Path, 50, true)
	err = d.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = b.Close()
	if err != nil {
		t.Fatal(err)
	}
}

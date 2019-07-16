// +build linux

package overlay

import (
	"context"
	"errors"
	"os"
	"testing"
)

type undo struct {
	osMkdirAll  func(string, os.FileMode) error
	osRemoveAll func(string) error
	unixMount   func(string, string, string, uintptr, string) error
}

func (u *undo) Close() {
	osMkdirAll = u.osMkdirAll
	unixMount = u.unixMount
}

// Captures the actual product function context and returns them on `Close()`.
// It sets the test dependencies to `nil` so that any unpredicted call to that
// function will cause the test to panic.
func captureTestMethods() *undo {
	u := &undo{
		osMkdirAll:  osMkdirAll,
		osRemoveAll: osRemoveAll,
		unixMount:   unixMount,
	}
	osMkdirAll = nil
	osRemoveAll = nil
	unixMount = nil
	return u
}

func Test_Mount_Success(t *testing.T) {
	undo := captureTestMethods()
	defer undo.Close()

	var upperCreated, workCreated, rootCreated bool
	osMkdirAll = func(path string, perm os.FileMode) error {
		if perm != 0755 {
			t.Errorf("os.MkdirAll at: %s, perm: %v expected perm: 0755", path, perm)
		}
		switch path {
		case "/upper":
			upperCreated = true
			return nil
		case "/work":
			workCreated = true
			return nil
		case "/root":
			rootCreated = true
			return nil
		}
		return errors.New("unexpected os.MkdirAll path")
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		if source != "overlay" {
			t.Errorf("expected source: 'overlay' got: %v", source)
		}
		if target != "/root" {
			t.Errorf("expected target: '/root' got: %v", target)
		}
		if fstype != "overlay" {
			t.Errorf("expected fstype: 'overlay' got: %v", fstype)
		}
		if flags != 0 {
			t.Errorf("expected flags: '0' got: %v", flags)
		}
		if data != "lowerdir=/layer1:/layer2,upperdir=/upper,workdir=/work" {
			t.Errorf("expected data: 'lowerdir=/layer1:/layer2,upperdir=/upper,workdir=/work' got: %v", data)
		}
		return nil
	}

	err := Mount(context.Background(), []string{"/layer1", "/layer2"}, "/upper", "/work", "/root", false)
	if err != nil {
		t.Fatalf("expected no error got: %v", err)
	}
	if !upperCreated || !workCreated || !rootCreated {
		t.Fatalf("expected all upper: %v, work: %v, root: %v to be created", upperCreated, workCreated, rootCreated)
	}
}

func Test_Mount_Readonly_Success(t *testing.T) {
	undo := captureTestMethods()
	defer undo.Close()

	var rootCreated bool
	osMkdirAll = func(path string, perm os.FileMode) error {
		if perm != 0755 {
			t.Errorf("os.MkdirAll at: %s, perm: %v expected perm: 0755", path, perm)
		}
		switch path {
		case "/root":
			rootCreated = true
			return nil
		}
		return errors.New("unexpected os.MkdirAll path")
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		if source != "overlay" {
			t.Errorf("expected source: 'overlay' got: %v", source)
		}
		if target != "/root" {
			t.Errorf("expected target: '/root' got: %v", target)
		}
		if fstype != "overlay" {
			t.Errorf("expected fstype: 'overlay' got: %v", fstype)
		}
		if flags != 0 {
			t.Errorf("expected flags: '0' got: %v", flags)
		}
		if data != "lowerdir=/layer1:/layer2" {
			t.Errorf("expected data: 'lowerdir=/layer1:/layer2' got: %v", data)
		}
		return nil
	}

	err := Mount(context.Background(), []string{"/layer1", "/layer2"}, "", "", "/root", false)
	if err != nil {
		t.Fatalf("expected no error got: %v", err)
	}
	if !rootCreated {
		t.Fatal("expected root to be created")
	}
}

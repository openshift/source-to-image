// +build linux

package storage

import (
	"os"
	"testing"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

func clearTestDependencies() {
	osStat = nil
	unixUnmount = nil
	osRemoveAll = nil
}

func Test_Unmount_Stat_Valid_Path(t *testing.T) {
	clearTestDependencies()

	expectedName := "/dev/fake"
	osStat = func(name string) (os.FileInfo, error) {
		if expectedName != name {
			t.Errorf("expected name: %s, got: %s", expectedName, name)
			return nil, errors.New("unexpected name")
		}
		return nil, nil
	}
	unixUnmount = func(target string, flags int) error {
		return nil
	}
	err := UnmountPath(expectedName, false)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func Test_Unmount_Stat_NotExist(t *testing.T) {
	clearTestDependencies()

	// Should return early
	osStat = func(name string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}
	err := UnmountPath("/dev/fake", false)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func Test_Unmount_Stat_OtherError_Error(t *testing.T) {
	clearTestDependencies()

	expectedErr := errors.New("expected stat err")
	osStat = func(name string) (os.FileInfo, error) {
		return nil, expectedErr
	}
	err := UnmountPath("/dev/fake", false)
	if errors.Cause(err) != expectedErr {
		t.Fatalf("expected err: %v, got: %v", expectedErr, err)
	}
}

func Test_Unmount_Valid_Target(t *testing.T) {
	clearTestDependencies()

	osStat = func(name string) (os.FileInfo, error) {
		return nil, nil
	}
	expectedTarget := "/dev/fake"
	unixUnmount = func(target string, flags int) error {
		if expectedTarget != target {
			t.Errorf("expected target: %s, got: %s", expectedTarget, target)
			return errors.New("unexpected target")
		}
		return nil
	}
	err := UnmountPath(expectedTarget, false)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func Test_Unmount_Valid_Flags(t *testing.T) {
	clearTestDependencies()

	osStat = func(name string) (os.FileInfo, error) {
		return nil, nil
	}
	unixUnmount = func(target string, flags int) error {
		if 0 != flags {
			t.Errorf("expected flags 0, got: %d", flags)
			return errors.New("unexpected flags")
		}
		return nil
	}
	err := UnmountPath("/fake/path", false)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func Test_Unmount_NotMounted(t *testing.T) {
	clearTestDependencies()

	osStat = func(name string) (os.FileInfo, error) {
		return nil, nil
	}
	unixUnmount = func(target string, flags int) error {
		return unix.EINVAL
	}
	err := UnmountPath("/dev/fake", false)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func Test_Unmount_OtherError(t *testing.T) {
	clearTestDependencies()

	osStat = func(name string) (os.FileInfo, error) {
		return nil, nil
	}
	expectedErr := errors.New("expected unmount error")
	unixUnmount = func(target string, flags int) error {
		return expectedErr
	}
	err := UnmountPath("/dev/fake", false)
	if errors.Cause(err) != expectedErr {
		t.Fatalf("expected err: %v, got: %v", expectedErr, err)
	}
}

func Test_Unmount_RemoveAll_Valid_Path(t *testing.T) {
	clearTestDependencies()

	osStat = func(name string) (os.FileInfo, error) {
		return nil, nil
	}
	unixUnmount = func(target string, flags int) error {
		return nil
	}
	expectedPath := "/fake/path"
	osRemoveAll = func(path string) error {
		if expectedPath != path {
			t.Errorf("expected path %s, got: %s", expectedPath, path)
			return errors.New("unexpected path")
		}
		return nil
	}
	err := UnmountPath(expectedPath, true)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func Test_Unmount_RemoveAll_Called(t *testing.T) {
	clearTestDependencies()

	osStat = func(name string) (os.FileInfo, error) {
		return nil, nil
	}
	unixUnmount = func(target string, flags int) error {
		return nil
	}
	removeAllCalled := false
	osRemoveAll = func(path string) error {
		removeAllCalled = true
		return nil
	}
	err := UnmountPath("/fake/path", true)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if !removeAllCalled {
		t.Fatal("expected remove to be called")
	}
}

// +build linux

package scsi

import (
	"context"
	"errors"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func clearTestDependencies() {
	osMkdirAll = nil
	osRemoveAll = nil
	unixMount = nil
	controllerLunToName = nil
}

func Test_Mount_Mkdir_Fails_Error(t *testing.T) {
	clearTestDependencies()

	expectedErr := errors.New("mkdir : no such file or directory")
	osMkdirAll = func(path string, perm os.FileMode) error {
		return expectedErr
	}
	err := Mount(context.Background(), 0, 0, "", false)
	if err != expectedErr {
		t.Fatalf("expected err: %v, got: %v", expectedErr, err)
	}
}

func Test_Mount_Mkdir_ExpectedPath(t *testing.T) {
	clearTestDependencies()

	// NOTE: Do NOT set osRemoveAll because the mount succeeds. Expect it not to
	// be called.

	target := "/fake/path"
	osMkdirAll = func(path string, perm os.FileMode) error {
		if path != target {
			t.Errorf("expected path: %v, got: %v", target, path)
			return errors.New("unexpected path")
		}
		return nil
	}
	controllerLunToName = func(ctx context.Context, controller, lun uint8) (string, error) {
		return "", nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		// Fake the mount success
		return nil
	}
	err := Mount(context.Background(), 0, 0, target, false)
	if err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}
}

func Test_Mount_Mkdir_ExpectedPerm(t *testing.T) {
	clearTestDependencies()

	// NOTE: Do NOT set osRemoveAll because the mount succeeds. Expect it not to
	// be called.

	target := "/fake/path"
	osMkdirAll = func(path string, perm os.FileMode) error {
		if perm != os.FileMode(0700) {
			t.Errorf("expected perm: %v, got: %v", os.FileMode(0700), perm)
			return errors.New("unexpected perm")
		}
		return nil
	}
	controllerLunToName = func(ctx context.Context, controller, lun uint8) (string, error) {
		return "", nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		// Fake the mount success
		return nil
	}
	err := Mount(context.Background(), 0, 0, target, false)
	if err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}
}

func Test_Mount_ControllerLunToName_Valid_Controller(t *testing.T) {
	clearTestDependencies()

	// NOTE: Do NOT set osRemoveAll because the mount succeeds. Expect it not to
	// be called.

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	expectedController := uint8(2)
	controllerLunToName = func(ctx context.Context, controller, lun uint8) (string, error) {
		if expectedController != controller {
			t.Errorf("expected controller: %v, got: %v", expectedController, controller)
			return "", errors.New("unexpected controller")
		}
		return "", nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		// Fake the mount success
		return nil
	}
	err := Mount(context.Background(), expectedController, 0, "/fake/path", false)
	if err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}
}

func Test_Mount_ControllerLunToName_Valid_Lun(t *testing.T) {
	clearTestDependencies()

	// NOTE: Do NOT set osRemoveAll because the mount succeeds. Expect it not to
	// be called.

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	expectedLun := uint8(2)
	controllerLunToName = func(ctx context.Context, controller, lun uint8) (string, error) {
		if expectedLun != lun {
			t.Errorf("expected lun: %v, got: %v", expectedLun, lun)
			return "", errors.New("unexpected lun")
		}
		return "", nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		// Fake the mount success
		return nil
	}
	err := Mount(context.Background(), 0, expectedLun, "/fake/path", false)
	if err != nil {
		t.Fatalf("expected nil error got: %v", err)
	}
}

func Test_Mount_Calls_RemoveAll_OnControllerToLunFailure(t *testing.T) {
	clearTestDependencies()

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	expectedErr := errors.New("expected controller to lun failure")
	controllerLunToName = func(ctx context.Context, controller, lun uint8) (string, error) {
		return "", expectedErr
	}
	target := "/fake/path"
	removeAllCalled := false
	osRemoveAll = func(path string) error {
		removeAllCalled = true
		if path != target {
			t.Errorf("expected path: %v, got: %v", target, path)
			return errors.New("unexpected path")
		}
		return nil
	}

	// NOTE: Do NOT set unixMount because the controller to lun fails. Expect it
	// not to be called.

	err := Mount(context.Background(), 0, 0, target, false)
	if err != expectedErr {
		t.Fatalf("expected err: %v, got: %v", expectedErr, err)
	}
	if !removeAllCalled {
		t.Fatal("expected os.RemoveAll to be called on mount failure")
	}
}

func Test_Mount_Calls_RemoveAll_OnMountFailure(t *testing.T) {
	clearTestDependencies()

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	controllerLunToName = func(ctx context.Context, controller, lun uint8) (string, error) {
		return "", nil
	}
	target := "/fake/path"
	removeAllCalled := false
	osRemoveAll = func(path string) error {
		removeAllCalled = true
		if path != target {
			t.Errorf("expected path: %v, got: %v", target, path)
			return errors.New("unexpected path")
		}
		return nil
	}
	expectedErr := errors.New("unexpected mount failure")
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		// Fake the mount failure to test remove is called
		return expectedErr
	}
	err := Mount(context.Background(), 0, 0, target, false)
	if err != expectedErr {
		t.Fatalf("expected err: %v, got: %v", expectedErr, err)
	}
	if !removeAllCalled {
		t.Fatal("expected os.RemoveAll to be called on mount failure")
	}
}

func Test_Mount_Valid_Source(t *testing.T) {
	clearTestDependencies()

	// NOTE: Do NOT set osRemoveAll because the mount succeeds. Expect it not to
	// be called.

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	expectedSource := "/dev/sdz"
	controllerLunToName = func(ctx context.Context, controller, lun uint8) (string, error) {
		return expectedSource, nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		if expectedSource != source {
			t.Errorf("expected source: %s, got: %s", expectedSource, source)
			return errors.New("unexpected source")
		}
		return nil
	}
	err := Mount(context.Background(), 0, 0, "/fake/path", false)
	if err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}
}

func Test_Mount_Valid_Target(t *testing.T) {
	clearTestDependencies()

	// NOTE: Do NOT set osRemoveAll because the mount succeeds. Expect it not to
	// be called.

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	controllerLunToName = func(ctx context.Context, controller, lun uint8) (string, error) {
		return "", nil
	}
	expectedTarget := "/fake/path"
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		if expectedTarget != target {
			t.Errorf("expected target: %s, got: %s", expectedTarget, target)
			return errors.New("unexpected target")
		}
		return nil
	}
	err := Mount(context.Background(), 0, 0, expectedTarget, false)
	if err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}
}

func Test_Mount_Valid_FSType(t *testing.T) {
	clearTestDependencies()

	// NOTE: Do NOT set osRemoveAll because the mount succeeds. Expect it not to
	// be called.

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	controllerLunToName = func(ctx context.Context, controller, lun uint8) (string, error) {
		return "", nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		expectedFSType := "ext4"
		if expectedFSType != fstype {
			t.Errorf("expected fstype: %s, got: %s", expectedFSType, fstype)
			return errors.New("unexpected fstype")
		}
		return nil
	}
	err := Mount(context.Background(), 0, 0, "/fake/path", false)
	if err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}
}

func Test_Mount_Valid_Flags(t *testing.T) {
	clearTestDependencies()

	// NOTE: Do NOT set osRemoveAll because the mount succeeds. Expect it not to
	// be called.

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	controllerLunToName = func(ctx context.Context, controller, lun uint8) (string, error) {
		return "", nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		expectedFlags := uintptr(0)
		if expectedFlags != flags {
			t.Errorf("expected flags: %v, got: %v", expectedFlags, flags)
			return errors.New("unexpected flags")
		}
		return nil
	}
	err := Mount(context.Background(), 0, 0, "/fake/path", false)
	if err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}
}

func Test_Mount_Readonly_Valid_Flags(t *testing.T) {
	clearTestDependencies()

	// NOTE: Do NOT set osRemoveAll because the mount succeeds. Expect it not to
	// be called.

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	controllerLunToName = func(ctx context.Context, controller, lun uint8) (string, error) {
		return "", nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		expectedFlags := uintptr(unix.MS_RDONLY)
		if expectedFlags != flags {
			t.Errorf("expected flags: %v, got: %v", expectedFlags, flags)
			return errors.New("unexpected flags")
		}
		return nil
	}
	err := Mount(context.Background(), 0, 0, "/fake/path", true)
	if err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}
}

func Test_Mount_Valid_Data(t *testing.T) {
	clearTestDependencies()

	// NOTE: Do NOT set osRemoveAll because the mount succeeds. Expect it not to
	// be called.

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	controllerLunToName = func(ctx context.Context, controller, lun uint8) (string, error) {
		return "", nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		if "" != data {
			t.Errorf("expected empty data, got: %s", data)
			return errors.New("unexpected data")
		}
		return nil
	}
	err := Mount(context.Background(), 0, 0, "/fake/path", false)
	if err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}
}

func Test_Mount_Readonly_Valid_Data(t *testing.T) {
	clearTestDependencies()

	// NOTE: Do NOT set osRemoveAll because the mount succeeds. Expect it not to
	// be called.

	osMkdirAll = func(path string, perm os.FileMode) error {
		return nil
	}
	controllerLunToName = func(ctx context.Context, controller, lun uint8) (string, error) {
		return "", nil
	}
	unixMount = func(source string, target string, fstype string, flags uintptr, data string) error {
		expectedData := "noload"
		if expectedData != data {
			t.Errorf("expected data: %s, got: %s", expectedData, data)
			return errors.New("unexpected data")
		}
		return nil
	}
	err := Mount(context.Background(), 0, 0, "/fake/path", true)
	if err != nil {
		t.Fatalf("expected nil err, got: %v", err)
	}
}

package remotefs

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io/ioutil"
	"os"
	"reflect"
	"syscall"
	"testing"

	"github.com/docker/docker/pkg/archive"
)

const (
	PathErrorCase          = "PathError"
	LinkErrorCase          = "LinkError"
	SyscallErrorCase       = "SyscallError"
	GenericErrorStringCase = "GenericErrorString"
	ErrNotExistCase        = "ErrNotExist"
)

// dummy error for testing
var dummySyscallError = syscall.Errno(100) // ENONET

var errorCases = map[string]error{
	PathErrorCase: &os.PathError{
		Op:   "fakePathOp",
		Path: "fakePath",
		Err:  dummySyscallError, // Avoid the fixOSError change.
	},
	LinkErrorCase: &os.LinkError{
		Op:  "fakeLinkOp",
		Old: "fakeOldPath",
		New: "fakeNewPath",
		Err: dummySyscallError, // Avoid the fixOSError change.
	},
	SyscallErrorCase: &os.SyscallError{
		Syscall: "fakeSyscallOp",
		Err:     dummySyscallError, // Avoid the fixOSError change.
	},
	GenericErrorStringCase: &ExportedError{
		ErrString: "fakeGenericError",
	},
	ErrNotExistCase: &os.PathError{
		Op:   "fakePathOp",
		Path: "fakePath",
		Err:  syscall.Errno(2), // Should get change to a generic error
	},
}

func sliceEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func testError(input error, expectedExported *ExportedError, t *testing.T) {
	buf := &bytes.Buffer{}
	err := WriteError(input, buf)
	if err != nil {
		t.Errorf("failed to write error: %s", err)
	}

	expectedBuf, err := json.Marshal(expectedExported)
	if err != nil {
		t.Errorf("failed to marshall json: %s", err)
	}

	if !sliceEqual(buf.Bytes(), expectedBuf) {
		t.Errorf("expected: %s. got %s", expectedBuf, buf.Bytes())
	}

	exported, err := ReadError(buf)
	if err != nil {
		t.Errorf("failed to export error: %s", err)
	}

	if exported == nil {
		t.Errorf("failed: got nil error instead of %+v", expectedExported)
	}

	if *exported != *expectedExported {
		t.Errorf("expected %#v, got %#v", expectedExported, exported)
	}
}

func TestPathError(t *testing.T) {
	input := errorCases[PathErrorCase]
	expected := &ExportedError{
		ErrString: input.Error(),
		ErrNum:    int(dummySyscallError),
	}
	testError(input, expected, t)
}

func TestLinkError(t *testing.T) {
	input := errorCases[LinkErrorCase]
	expected := &ExportedError{
		ErrString: input.Error(),
		ErrNum:    int(dummySyscallError),
	}
	testError(input, expected, t)
}

func TestSyscallError(t *testing.T) {
	input := errorCases[SyscallErrorCase]
	expected := &ExportedError{
		ErrString: input.Error(),
		ErrNum:    int(dummySyscallError),
	}
	testError(input, expected, t)
}

func TestGenericError(t *testing.T) {
	input := errorCases[GenericErrorStringCase]
	expected := &ExportedError{
		ErrString: input.Error(),
	}
	testError(input, expected, t)
}

func TestPathToGenericError(t *testing.T) {
	input := errorCases[ErrNotExistCase]
	expectedExported := &ExportedError{
		ErrString: os.ErrNotExist.Error(),
	}
	testError(input, expectedExported, t)
}

func TestStat(t *testing.T) {
	file, err := ioutil.TempFile("", "TestStat")
	if err != nil {
		t.Errorf("failed to create temp file")
	}
	file.Close()

	buf := &bytes.Buffer{}
	err = Stat(nil, buf, []string{file.Name()})
	if err != nil {
		os.Remove(file.Name())
		t.Errorf("failed to stat: %s", err)
	}

	fiExp, err := os.Stat(file.Name())
	if err != nil {
		os.Remove(file.Name())
		t.Errorf("failed to stat: %s", err)
	}

	var fiAct FileInfo
	if err := json.Unmarshal(buf.Bytes(), &fiAct); err != nil {
		os.Remove(file.Name())
		t.Errorf("failed to unmarshal: %s", err)
	}

	os.Remove(file.Name())
	if fiExp.Name() != fiAct.Name() ||
		fiExp.Size() != fiAct.Size() ||
		fiExp.Mode() != fiAct.Mode() ||
		fiExp.IsDir() != fiAct.IsDir() ||
		fiExp.ModTime() != fiAct.ModTime() {
		t.Errorf("failed to get right stat: %#v expected, actual %#v", fiExp, fiAct)
	}
}

func TestTar(t *testing.T) {
	opts := &archive.TarOptions{}
	expectedBytes, err := json.Marshal(opts)
	if err != nil {
		t.Errorf("failed to marshal json: %s", err)
	}
	expectedSize := len(expectedBytes) + 8

	buf := &bytes.Buffer{}
	if err := WriteTarOptions(buf, opts); err != nil {
		t.Errorf("failed to write tar opts: %s", err)
	}

	if expectedSize != buf.Len() {
		t.Errorf("unexpected buffer size for tar: expected %d. got %d", expectedSize, buf.Len())
	}

	if !sliceEqual(expectedBytes, buf.Bytes()[8:]) {
		t.Errorf("unexpected tar serialization: expected %v. got %v", expectedBytes, buf.Bytes()[8:])
	}

	if actualSize := binary.BigEndian.Uint64(buf.Bytes()[:8]); actualSize != uint64(expectedSize-8) {
		t.Errorf("unexpected tar size: expected %d, got %d", expectedSize, actualSize)
	}

	opts2, err := ReadTarOptions(buf)
	if err != nil {
		t.Errorf("error reading tar opts: %s", err)
	}

	if !reflect.DeepEqual(opts, opts2) {
		t.Errorf("error. tar opts is different. expected: %+v, got %+v", opts, opts2)
	}
}

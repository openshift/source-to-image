// +build go1.12

/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package fifo

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

func TestRawReadWrite(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "fifos")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpdir)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	r, err := OpenFifo(ctx, filepath.Join(tmpdir, t.Name()), syscall.O_RDONLY|syscall.O_CREAT|syscall.O_NONBLOCK, 0600)
	assert.NoError(t, err)
	defer r.Close()
	rawR := makeRawConn(t, r, false)
	assert.Error(t, rawR.Write(func(uintptr) bool { return true }))

	w, err := OpenFifo(ctx, filepath.Join(tmpdir, t.Name()), syscall.O_WRONLY|syscall.O_NONBLOCK, 0)
	assert.NoError(t, err)
	defer w.Close()
	rawW := makeRawConn(t, w, false)
	assert.Error(t, rawW.Read(func(uintptr) bool { return true }))

	data := []byte("hello world")
	rawWrite(t, rawW, data)

	dataR := make([]byte, len(data))
	rawRead(t, rawR, dataR)
	assert.True(t, bytes.Equal(data, dataR))
}

func TestRawWriteUserRead(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "fifos")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpdir)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	w, err := OpenFifo(ctx, filepath.Join(tmpdir, t.Name()), syscall.O_WRONLY|syscall.O_CREAT|syscall.O_NONBLOCK, 0600)
	assert.NoError(t, err)
	defer w.Close()
	rawW := makeRawConn(t, w, false)

	r, err := OpenFifo(ctx, filepath.Join(tmpdir, t.Name()), syscall.O_RDONLY|syscall.O_CREAT|syscall.O_NONBLOCK, 0600)
	assert.NoError(t, err)
	defer r.Close()

	data := []byte("hello world!")
	rawWrite(t, rawW, data)
	w.Close()

	buf := make([]byte, len(data))
	n, err := io.ReadFull(r, buf)
	assert.NoError(t, err)
	assert.True(t, bytes.Equal(data, buf[:n]))
}

func TestUserWriteRawRead(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "fifos")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpdir)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	w, err := OpenFifo(ctx, filepath.Join(tmpdir, t.Name()), syscall.O_WRONLY|syscall.O_CREAT|syscall.O_NONBLOCK, 0600)
	assert.NoError(t, err)
	defer w.Close()

	r, err := OpenFifo(ctx, filepath.Join(tmpdir, t.Name()), syscall.O_RDONLY|syscall.O_CREAT|syscall.O_NONBLOCK, 0600)
	assert.NoError(t, err)
	defer r.Close()
	rawR := makeRawConn(t, r, false)

	data := []byte("hello world!")
	n, err := w.Write(data)
	assert.NoError(t, err)
	assert.Equal(t, n, len(data))
	w.Close()

	buf := make([]byte, len(data))
	rawRead(t, rawR, buf)
	assert.True(t, bytes.Equal(data, buf[:n]))
}

func TestRawCloseError(t *testing.T) {
	tmpdir, err := ioutil.TempDir("", "fifos")
	assert.NoError(t, err)
	defer os.RemoveAll(tmpdir)

	t.Run("SyscallConnAfterClose", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		f, err := OpenFifo(ctx, filepath.Join(tmpdir, path.Base(t.Name())), syscall.O_RDWR|syscall.O_CREAT, 0600)
		assert.NoError(t, err)

		f.Close()

		makeRawConn(t, f, true)
	})

	t.Run("RawOpsAfterClose", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		f, err := OpenFifo(ctx, filepath.Join(tmpdir, path.Base(t.Name())), syscall.O_RDWR|syscall.O_CREAT, 0600)
		assert.NoError(t, err)
		defer f.Close()

		raw := makeRawConn(t, f, false)

		f.Close()

		assert.Error(t, raw.Control(func(uintptr) {}))
		dummy := func(uintptr) bool { return true }
		assert.Error(t, raw.Write(dummy))
		assert.Error(t, raw.Read(dummy))
	})
}

func makeRawConn(t *testing.T, fifo io.ReadWriteCloser, expectError bool) syscall.RawConn {
	sc, ok := fifo.(syscall.Conn)
	assert.True(t, ok, "not a syscall.Conn")

	raw, err := sc.SyscallConn()
	if !expectError {
		assert.NoError(t, err)
	} else {
		assert.Error(t, err)
	}

	return raw
}

func rawWrite(t *testing.T, rc syscall.RawConn, data []byte) {
	var written int
	var wErr error

	err := rc.Write(func(fd uintptr) bool {
		var n int
		n, wErr = syscall.Write(int(fd), data[written:])
		written += n
		if wErr != nil || n == 0 || written == len(data) {
			return true
		}
		return false
	})
	assert.NoError(t, err)
	assert.NoError(t, wErr)
	assert.Equal(t, written, len(data))
}

func rawRead(t *testing.T, rc syscall.RawConn, data []byte) {
	var (
		rErr error
		read int
	)

	err := rc.Read(func(fd uintptr) bool {
		var n int
		n, rErr = syscall.Read(int(fd), data[read:])
		read += n
		if rErr != nil || n == 0 || read == len(data) {
			return true
		}
		return false
	})
	assert.NoError(t, err)
	assert.NoError(t, rErr)
	assert.Equal(t, read, len(data))
}

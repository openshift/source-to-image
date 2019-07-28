package fs

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	utillog "github.com/openshift/source-to-image/pkg/util/log"

	s2ierr "github.com/openshift/source-to-image/pkg/errors"
)

var log = utillog.StderrLog

// FileSystem allows STI to work with the file system and
// perform tasks such as creating and deleting directories
type FileSystem interface {
	Chmod(file string, mode os.FileMode) error
	Rename(from, to string) error
	MkdirAll(dirname string) error
	MkdirAllWithPermissions(dirname string, perm os.FileMode) error
	Mkdir(dirname string) error
	Exists(file string) bool
	Copy(sourcePath, targetPath string, filesToIgnore map[string]string) error
	CopyContents(sourcePath, targetPath string, filesToIgnore map[string]string) error
	RemoveDirectory(dir string) error
	CreateWorkingDirectory() (string, error)
	Open(file string) (io.ReadCloser, error)
	Create(file string) (io.WriteCloser, error)
	WriteFile(file string, data []byte) error
	ReadDir(string) ([]os.FileInfo, error)
	Stat(string) (os.FileInfo, error)
	Lstat(string) (os.FileInfo, error)
	Walk(string, filepath.WalkFunc) error
	Readlink(string) (string, error)
	Symlink(string, string) error
	KeepSymlinks(bool)
	ShouldKeepSymlinks() bool
}

// NewFileSystem creates a new instance of the default FileSystem
// implementation
func NewFileSystem() FileSystem {
	return &fs{
		fileModes:    make(map[string]os.FileMode),
		keepSymlinks: false,
	}
}

type fs struct {
	// on Windows, fileModes is used to track the UNIX file mode of every file we
	// work with; m is used to synchronize access to fileModes.
	fileModes    map[string]os.FileMode
	m            sync.Mutex
	keepSymlinks bool
}

// FileInfo is a struct which implements os.FileInfo.  We use it (a) for test
// purposes, and (b) because we enrich the FileMode on Windows systems
type FileInfo struct {
	FileName    string
	FileSize    int64
	FileMode    os.FileMode
	FileModTime time.Time
	FileIsDir   bool
	FileSys     interface{}
}

// Name retuns the filename of fi
func (fi *FileInfo) Name() string {
	return fi.FileName
}

// Size returns the file size of fi
func (fi *FileInfo) Size() int64 {
	return fi.FileSize
}

// Mode returns the file mode of fi
func (fi *FileInfo) Mode() os.FileMode {
	return fi.FileMode
}

// ModTime returns the file modification time of fi
func (fi *FileInfo) ModTime() time.Time {
	return fi.FileModTime
}

// IsDir returns true if fi refers to a directory
func (fi *FileInfo) IsDir() bool {
	return fi.FileIsDir
}

// Sys returns the sys interface of fi
func (fi *FileInfo) Sys() interface{} {
	return fi.FileSys
}

func copyFileInfo(src os.FileInfo) *FileInfo {
	return &FileInfo{
		FileName:    src.Name(),
		FileSize:    src.Size(),
		FileMode:    src.Mode(),
		FileModTime: src.ModTime(),
		FileIsDir:   src.IsDir(),
		FileSys:     src.Sys(),
	}
}

// Stat returns a FileInfo describing the named file.
func (h *fs) Stat(path string) (os.FileInfo, error) {
	fi, err := os.Stat(path)
	if runtime.GOOS == "windows" && err == nil {
		fi = h.enrichFileInfo(path, fi)
	}
	return fi, err
}

// Lstat returns a FileInfo describing the named file (not following symlinks).
func (h *fs) Lstat(path string) (os.FileInfo, error) {
	fi, err := os.Lstat(path)
	if runtime.GOOS == "windows" && err == nil {
		fi = h.enrichFileInfo(path, fi)
	}
	return fi, err
}

// ReadDir reads the directory named by dirname and returns a list of directory
// entries sorted by filename.
func (h *fs) ReadDir(path string) ([]os.FileInfo, error) {
	fis, err := ioutil.ReadDir(path)
	if runtime.GOOS == "windows" && err == nil {
		h.enrichFileInfos(path, fis)
	}
	return fis, err
}

// Chmod sets the file mode
func (h *fs) Chmod(file string, mode os.FileMode) error {
	err := os.Chmod(file, mode)
	if runtime.GOOS == "windows" && err == nil {
		h.m.Lock()
		h.fileModes[file] = mode
		h.m.Unlock()
		return nil
	}
	return err
}

// Rename renames or moves a file
func (h *fs) Rename(from, to string) error {
	return os.Rename(from, to)
}

// MkdirAll creates the directory and all its parents
func (h *fs) MkdirAll(dirname string) error {
	return os.MkdirAll(dirname, 0700)
}

// MkdirAllWithPermissions creates the directory and all its parents with the provided permissions
func (h *fs) MkdirAllWithPermissions(dirname string, perm os.FileMode) error {
	return os.MkdirAll(dirname, perm)
}

// Mkdir creates the specified directory
func (h *fs) Mkdir(dirname string) error {
	return os.Mkdir(dirname, 0700)
}

// Exists determines whether the given file exists
func (h *fs) Exists(file string) bool {
	_, err := h.Stat(file)
	return err == nil
}

// Copy copies the source to a destination.
// If the source is a file, then the destination has to be a file as well,
// otherwise you will get an error.
// If the source is a directory, then the destination has to be a directory and
// we copy the content of the source directory to destination directory
// recursively.
func (h *fs) Copy(source string, dest string, filesToIgnore map[string]string) (err error) {
	return doCopy(h, source, dest, filesToIgnore)
}

// KeepSymlinks configures fs to copy symlinks from src as symlinks to dst.
// Default behavior is to follow symlinks and copy files by content.
func (h *fs) KeepSymlinks(k bool) {
	h.keepSymlinks = k
}

// ShouldKeepSymlinks is exported only due to the design of fs util package
// and how the tests are structured. It indicates whether the implementation
// should copy symlinks as symlinks or follow symlinks and copy by content.
func (h *fs) ShouldKeepSymlinks() bool {
	return h.keepSymlinks
}

// If src is symlink and symlink copy has been enabled, copy as a symlink.
// Otherwise ignore symlink and let rest of the code follow the symlink
// and copy the content of the file
func handleSymlink(h FileSystem, source, dest string) (bool, error) {
	lstatinfo, lstaterr := h.Lstat(source)
	_, staterr := h.Stat(source)
	if lstaterr == nil &&
		lstatinfo.Mode()&os.ModeSymlink != 0 {
		if os.IsNotExist(staterr) {
			log.V(5).Infof("(broken) L %q -> %q", source, dest)
		} else if h.ShouldKeepSymlinks() {
			log.V(5).Infof("L %q -> %q", source, dest)
		} else {
			// symlink not handled here, will copy the file content
			return false, nil
		}
		linkdest, err := h.Readlink(source)
		if err != nil {
			return true, err
		}
		return true, h.Symlink(linkdest, dest)
	}
	// symlink not handled here, will copy the file content
	return false, nil
}

func doCopy(h FileSystem, source, dest string, filesToIgnore map[string]string) error {
	if handled, err := handleSymlink(h, source, dest); handled || err != nil {
		return err
	}
	sourcefile, err := h.Open(source)
	if err != nil {
		return err
	}
	defer sourcefile.Close()
	sourceinfo, err := h.Stat(source)
	if err != nil {
		return err
	}

	if sourceinfo.IsDir() {
		_, ok := filesToIgnore[source]
		if ok {
			log.V(5).Infof("Directory %q ignored", source)
			return nil
		}
		log.V(5).Infof("D %q -> %q", source, dest)
		return h.CopyContents(source, dest, filesToIgnore)
	}

	destinfo, _ := h.Stat(dest)
	if destinfo != nil && destinfo.IsDir() {
		return fmt.Errorf("destination must be full path to a file, not directory")
	}
	destfile, err := h.Create(dest)
	if err != nil {
		return err
	}
	defer destfile.Close()
	_, ok := filesToIgnore[source]
	if ok {
		log.V(5).Infof("File %q ignored", source)
		return nil
	}
	log.V(5).Infof("F %q -> %q", source, dest)
	if _, err := io.Copy(destfile, sourcefile); err != nil {
		return err
	}

	return h.Chmod(dest, sourceinfo.Mode())
}

// CopyContents copies the content of the source directory to a destination
// directory.
// If the destination directory does not exists, it will be created.
// The source directory itself will not be copied, only its content. If you
// want this behavior, the destination must include the source directory name.
// It will skip any files provided in filesToIgnore from being copied
func (h *fs) CopyContents(src string, dest string, filesToIgnore map[string]string) (err error) {
	sourceinfo, err := h.Stat(src)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dest, sourceinfo.Mode()); err != nil {
		return err
	}
	directory, err := os.Open(src)
	if err != nil {
		return err
	}
	defer directory.Close()
	objects, err := directory.Readdir(-1)
	if err != nil {
		return err
	}

	for _, obj := range objects {
		source := path.Join(src, obj.Name())
		destination := path.Join(dest, obj.Name())
		if err := h.Copy(source, destination, filesToIgnore); err != nil {
			return err
		}
	}
	return
}

// RemoveDirectory removes the specified directory and all its contents
func (h *fs) RemoveDirectory(dir string) error {
	log.V(2).Infof("Removing directory '%s'", dir)

	// HACK: If deleting a directory in windows, call out to the system to do the deletion
	// TODO: Remove this workaround when we switch to go 1.7 -- os.RemoveAll should
	// be fixed for Windows in that release.  https://github.com/golang/go/issues/9606
	if runtime.GOOS == "windows" {
		command := exec.Command("cmd.exe", "/c", fmt.Sprintf("rd /s /q %s", dir))
		output, err := command.Output()
		if err != nil {
			log.Errorf("Error removing directory %q: %v %s", dir, err, string(output))
			return err
		}
		return nil
	}

	err := os.RemoveAll(dir)
	if err != nil {
		log.Errorf("Error removing directory '%s': %v", dir, err)
	}
	return err
}

// CreateWorkingDirectory creates a directory to be used for STI
func (h *fs) CreateWorkingDirectory() (directory string, err error) {
	directory, err = ioutil.TempDir("", "s2i")
	if err != nil {
		return "", s2ierr.NewWorkDirError(directory, err)
	}

	return directory, err
}

// Open opens a file and returns a ReadCloser interface to that file
func (h *fs) Open(filename string) (io.ReadCloser, error) {
	return os.Open(filename)
}

// Create creates a file and returns a WriteCloser interface to that file
func (h *fs) Create(filename string) (io.WriteCloser, error) {
	return os.Create(filename)
}

// WriteFile opens a file and writes data to it, returning error if such
// occurred
func (h *fs) WriteFile(filename string, data []byte) error {
	return ioutil.WriteFile(filename, data, 0700)
}

// Walk walks the file tree rooted at root, calling walkFn for each file or
// directory in the tree, including root.
func (h *fs) Walk(root string, walkFn filepath.WalkFunc) error {
	wrapper := func(path string, info os.FileInfo, err error) error {
		if runtime.GOOS == "windows" && err == nil {
			info = h.enrichFileInfo(path, info)
		}
		return walkFn(path, info, err)
	}
	return filepath.Walk(root, wrapper)
}

// enrichFileInfo is used on Windows.  It takes an os.FileInfo object, e.g. as
// returned by os.Stat, and enriches the OS-returned file mode with the "real"
// UNIX file mode, if we know what it is.
func (h *fs) enrichFileInfo(path string, fi os.FileInfo) os.FileInfo {
	h.m.Lock()
	if mode, ok := h.fileModes[path]; ok {
		fi = copyFileInfo(fi)
		fi.(*FileInfo).FileMode = mode
	}
	h.m.Unlock()
	return fi
}

// enrichFileInfos is used on Windows.  It takes an array of os.FileInfo
// objects, e.g. as returned by os.ReadDir, and for each file enriches the OS-
// returned file mode with the "real" UNIX file mode, if we know what it is.
func (h *fs) enrichFileInfos(root string, fis []os.FileInfo) {
	h.m.Lock()
	for i := range fis {
		if mode, ok := h.fileModes[filepath.Join(root, fis[i].Name())]; ok {
			fis[i] = copyFileInfo(fis[i])
			fis[i].(*FileInfo).FileMode = mode
		}
	}
	h.m.Unlock()
}

// Readlink reads the destination of a symlink
func (h *fs) Readlink(name string) (string, error) {
	return os.Readlink(name)
}

// Symlink creates a symlink at newname, pointing to oldname
func (h *fs) Symlink(oldname, newname string) error {
	return os.Symlink(oldname, newname)
}

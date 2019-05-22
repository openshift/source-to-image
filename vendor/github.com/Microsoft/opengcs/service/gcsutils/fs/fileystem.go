package fs

import "os"

// Filesystem is an interface that calculates the disk space a filesystem
// would take up and offers the option to create a file system disk image.
type Filesystem interface {
	// InitContext() starts a new context to calculate the file system size.
	InitSizeContext() error

	// These functions calculate the size of different Linux file types after
	// the context has been created.
	CalcRegFileSize(fileName string, fileSize uint64) error
	CalcDirSize(dirName string) error
	CalcSymlinkSize(srcName string, dstName string) error
	CalcHardlinkSize(srcName string, dstName string) error
	CalcCharDeviceSize(devName string, major uint64, minor uint64) error
	CalcBlockDeviceSize(devName string, major uint64, minor uint64) error
	CalcFIFOPipeSize(pipeName string) error
	CalcSocketSize(sockName string) error
	CalcAddExAttrSize(fileName string, attr string, data []byte, flags int) error

	// FinalizeContext() finalizes the total size and inodes for the filesystem
	// in the current context.
	FinalizeSizeContext() error

	// GetSizeInfo() can be called before FinalizeContext() to get a intermediate result
	// or after to get the final result.
	GetSizeInfo() FilesystemSizeInfo

	// MakeFileSystem takes the current context and creates a file system on the
	// given device.
	MakeFileSystem(file *os.File) error

	// CleanupContext() clears up system resources to safely begin a new context
	CleanupSizeContext() error
}

// FilesystemSizeInfo contains the number of inodes and the total size.
type FilesystemSizeInfo struct {
	NumInodes uint64
	TotalSize uint64
}

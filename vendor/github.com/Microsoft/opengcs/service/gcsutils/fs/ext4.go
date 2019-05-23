package fs

import (
	"os"
	"os/exec"
	"strconv"

	"github.com/sirupsen/logrus"
)

// Ext4Fs implements the Filesystem interface for ext4.
//
// Ext4Fs makes the following assumptions about the ext4 file system.
//  - No journal or GDT table
//	- Extent tree (instead of direct/indirect block addressing)
//	- Hash tree directories (instead of linear directories)
//	- Inline symlinks if < 60 chars, but no inline directories or reg files
//  - sparse_super ext4 flags, so superblocks backups are in powers of 3, 5, 7
// 	- Directory entries take 1 block (even though its not true)
//  - All regular files/symlinks <= 128MB
type Ext4Fs struct {
	BlockSize uint64
	InodeSize uint64
	totalSize uint64
	numInodes uint64
}

// InitSizeContext creates the context for a new ext4 filesystem context
// Before calling set e.BlockSize and e.InodeSize to the desired values.
func (e *Ext4Fs) InitSizeContext() error {
	e.numInodes = 11                                                // ext4 has 11 reserved inodes
	e.totalSize = maxU64(2048+e.numInodes*e.InodeSize, e.BlockSize) // boot sector + super block is 2k
	return nil
}

// CalcRegFileSize calculates the space taken by the given regular file on a ext4
// file system with extent trees.
func (e *Ext4Fs) CalcRegFileSize(fileName string, fileSize uint64) error {
	// 1 directory entry
	// 1 inode
	e.addInode()
	e.totalSize += e.BlockSize

	// Each extent can hold 32k blocks, so 32M of data, so 128MB can get held
	// in the 4 extends below the i_block.
	e.totalSize += alignN(fileSize, e.BlockSize)
	return nil
}

// CalcDirSize calculates the space taken by the given directory on a ext4
// file system with hash tree directories enabled.
func (e *Ext4Fs) CalcDirSize(dirName string) error {
	// 1 directory entry for parent.
	// 1 inode with 2 directory entries ("." & ".." as data
	e.addInode()
	e.totalSize += 3 * e.BlockSize
	return nil
}

// CalcSymlinkSize calculates the space taken by a symlink taking account for
// inline symlinks.
func (e *Ext4Fs) CalcSymlinkSize(srcName string, dstName string) error {
	e.addInode()
	if len(dstName) > 60 {
		// Not an inline symlink. The path is 1 extent max since MAX_PATH=4096
		e.totalSize += alignN(uint64(len(dstName)), e.BlockSize)
	}
	return nil
}

// CalcHardlinkSize calculates the space taken by a hardlink.
func (e *Ext4Fs) CalcHardlinkSize(srcName string, dstName string) error {
	// 1 directory entry (No additional inode)
	e.totalSize += e.BlockSize
	return nil
}

// CalcCharDeviceSize calculates the space taken by a char device.
func (e *Ext4Fs) CalcCharDeviceSize(devName string, major uint64, minor uint64) error {
	e.addInode()
	return nil
}

// CalcBlockDeviceSize calculates the space taken by a block device.
func (e *Ext4Fs) CalcBlockDeviceSize(devName string, major uint64, minor uint64) error {
	e.addInode()
	return nil
}

// CalcFIFOPipeSize calculates the space taken by a fifo pipe.
func (e *Ext4Fs) CalcFIFOPipeSize(pipeName string) error {
	e.addInode()
	return nil
}

// CalcSocketSize calculates the space taken by a socket.
func (e *Ext4Fs) CalcSocketSize(sockName string) error {
	e.addInode()
	return nil
}

// CalcAddExAttrSize calculates the space taken by extended attributes.
func (e *Ext4Fs) CalcAddExAttrSize(fileName string, attr string, data []byte, flags int) error {
	// Since xattr are stored in the inode, we don't use any more space
	return nil
}

// FinalizeSizeContext should be after all of the CalcXSize methods are done.
// It does some final size adjustments.
func (e *Ext4Fs) FinalizeSizeContext() error {
	// Final adjustments to the size + inode
	// There are more metadata like Inode Table, block table.
	// For now, add 10% more to the size to take account for it.
	e.totalSize = uint64(float64(e.totalSize) * 1.10)
	e.numInodes = uint64(float64(e.numInodes) * 1.10)

	// Align to 64 * blocksize
	if e.totalSize%(64*e.BlockSize) != 0 {
		e.totalSize = alignN(e.totalSize, 64*e.BlockSize)
	}
	return nil
}

// GetSizeInfo returns the size of the ext4 file system after the size context is finalized.
func (e *Ext4Fs) GetSizeInfo() FilesystemSizeInfo {
	return FilesystemSizeInfo{NumInodes: e.numInodes, TotalSize: e.totalSize}
}

// CleanupSizeContext frees any resources needed by the ext4 file system
func (e *Ext4Fs) CleanupSizeContext() error {
	// No resources need to be freed
	return nil
}

// MakeFileSystem writes an ext4 filesystem to the given file after the size context is finalized.
func (e *Ext4Fs) MakeFileSystem(file *os.File) error {
	logrus.WithFields(logrus.Fields{
		"blockSize": e.BlockSize,
		"inodeSize": e.InodeSize,
		"numInodes": e.numInodes,
		"totalSize": e.totalSize,
	}).Info("opengcs::Ext4Fs::MakeFileSystem - making file system mkfs.ext4")

	blockSize := strconv.FormatUint(e.BlockSize, 10)
	inodeSize := strconv.FormatUint(e.InodeSize, 10)
	numInodes := strconv.FormatUint(e.numInodes, 10)

	return exec.Command(
		"mkfs.ext4",
		"-O", "^has_journal,^resize_inode",
		"-N", numInodes,
		"-b", blockSize,
		"-I", inodeSize,
		"-F",
		file.Name()).Run()
}

func maxU64(x, y uint64) uint64 {
	if x > y {
		return x
	}
	return y
}

func alignN(n uint64, alignto uint64) uint64 {
	if n%alignto == 0 {
		return n
	}
	return n + alignto - n%alignto
}

func (e *Ext4Fs) addInode() {
	e.numInodes++
	e.totalSize += e.InodeSize
}

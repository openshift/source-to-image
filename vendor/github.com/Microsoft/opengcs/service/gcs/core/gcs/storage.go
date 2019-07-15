package gcs

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Microsoft/opengcs/internal/storage"
	"github.com/Microsoft/opengcs/internal/storage/overlay"
	"github.com/Microsoft/opengcs/internal/storage/scsi"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	// baseFilesPath is the path in the utility VM containing all the files
	// that will be used as the base layer for containers.
	baseFilesPath = "/tmp/base/"

	// mappedDiskMountTimeout is the amount of time before
	// mountMappedVirtualDisks will give up trying to mount a device.
	mappedDiskMountTimeout = time.Second * 2
)

type mountSpec struct {
	Source     string
	FileSystem string
	Flags      uintptr
	Options    []string
}

const (
	// From mount(8): Don't load the journal on mounting.  Note that if the
	// filesystem was not unmounted cleanly, skipping the journal replay will
	// lead to the filesystem containing inconsistencies that can lead to any
	// number of problems.
	mountOptionNoLoad = "noload"
	// Enable DAX mode. This turns off the local cache for the file system and
	// accesses the storage directly from host memory, reducing memory use
	// and increasing sharing across VMs. Only supported on vPMEM devices.
	mountOptionDax = "dax"

	// For now the file system is hard-coded
	defaultFileSystem = "ext4"
)

// Mount mounts the file system to the specified target.
func (ms *mountSpec) Mount(target string) error {
	options := strings.Join(ms.Options, ",")
	err := syscall.Mount(ms.Source, target, ms.FileSystem, ms.Flags, options)
	if err != nil {
		return errors.Wrapf(err, "mount %s %s %s 0x%x %s", ms.Source, target, ms.FileSystem, ms.Flags, options)
	}
	return nil
}

// MountWithTimedRetry attempts mounting multiple times up until the given
// timout. This is necessary because there is a span of time between when the
// device name becomes available under /sys/bus/scsi and when it appears under
// /dev. Once it appears under /dev, there is still a span of time before it
// becomes mountable. Retrying mount should succeed in mounting the device as
// long as it becomes mountable under /dev before the timeout.
func (ms *mountSpec) MountWithTimedRetry(target string) error {
	startTime := time.Now()
	for {
		err := ms.Mount(target)
		if err != nil {
			if time.Since(startTime) > mappedDiskMountTimeout {
				return errors.Wrapf(err, "failed to mount directory %s for mapped virtual disk device %s", target, ms.Source)
			}
		} else {
			break
		}
		time.Sleep(time.Millisecond * 10)
	}
	return nil
}

// getLayerMounts computes the mount specs for the scratch and layers.
func (c *gcsCore) getLayerMounts(scratch string, layers []prot.Layer) (scratchMount *mountSpec, layerMounts []*mountSpec, err error) {
	layerMounts = make([]*mountSpec, len(layers))
	for i, layer := range layers {
		deviceName, pmem, err := c.deviceIDToName(layer.Path)
		if err != nil {
			return nil, nil, err
		}
		options := []string{mountOptionNoLoad}
		if pmem {
			// PMEM devices support DAX and should use it
			options = append(options, mountOptionDax)
		}
		layerMounts[i] = &mountSpec{
			Source:     deviceName,
			FileSystem: defaultFileSystem,
			Flags:      syscall.MS_RDONLY,
			Options:    options,
		}
	}
	// An empty scratch value indicates no scratch space is to be attached.
	if scratch != "" {
		scratchDevice, _, err := c.deviceIDToName(scratch)
		if err != nil {
			return nil, nil, err
		}
		scratchMount = &mountSpec{
			Source:     scratchDevice,
			FileSystem: defaultFileSystem,
		}
	}

	return scratchMount, layerMounts, nil
}

// getMappedVirtualDiskMounts uses the Lun values in the given disks to
// retrieve their associated mount spec.
func (c *gcsCore) getMappedVirtualDiskMounts(disks []prot.MappedVirtualDisk) ([]*mountSpec, error) {
	devices := make([]*mountSpec, len(disks))
	for i, disk := range disks {
		device, err := c.scsiLunToName(disk.Lun)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get device name for mapped virtual disk %s, lun %d", disk.ContainerPath, disk.Lun)
		}
		flags := uintptr(0)
		var options []string
		if disk.ReadOnly {
			flags |= syscall.MS_RDONLY
			options = append(options, mountOptionNoLoad)
		}
		devices[i] = &mountSpec{
			Source:     device,
			FileSystem: defaultFileSystem,
			Flags:      flags,
			Options:    options,
		}
	}
	return devices, nil
}

// scsiLunToName finds the SCSI device with the given LUN. This assumes
// only one SCSI controller.
func (c *gcsCore) scsiLunToName(lun uint8) (string, error) {
	return scsi.ControllerLunToName(context.Background(), 0, lun)
}

// deviceIDToName converts a device ID (scsi:<lun> or pmem:<device#> to a
// device name (/dev/sd? or /dev/pmem?).
// For temporary compatibility, this also accepts just <lun> for SCSI devices.
func (c *gcsCore) deviceIDToName(id string) (device string, pmem bool, err error) {
	const (
		pmemPrefix = "pmem:"
		scsiPrefix = "scsi:"
	)

	if strings.HasPrefix(id, pmemPrefix) {
		return "/dev/pmem" + id[len(pmemPrefix):], true, nil
	}

	lunStr := id
	if strings.HasPrefix(id, scsiPrefix) {
		lunStr = id[len(scsiPrefix):]
	}

	if lun, err := strconv.ParseInt(lunStr, 10, 8); err == nil {
		name, err := c.scsiLunToName(uint8(lun))
		return name, false, err
	}

	return "", false, errors.Errorf("unknown device ID %s", id)
}

// mountMappedVirtualDisks mounts the given disks to the given directories,
// with the given options. The device names of each disk are given in a
// parallel slice.
func (c *gcsCore) mountMappedVirtualDisks(disks []prot.MappedVirtualDisk, mounts []*mountSpec) error {
	if len(disks) != len(mounts) {
		return errors.Errorf("disk and device slices were of different sizes. disks: %d, mounts: %d", len(disks), len(mounts))
	}
	for i, disk := range disks {
		// Don't mount the disk if AttachOnly is specified.
		if !disk.AttachOnly {
			if !disk.CreateInUtilityVM {
				return errors.New("we do not currently support mapping virtual disks inside the container namespace")
			}
			mount := mounts[i]
			if err := os.MkdirAll(disk.ContainerPath, 0700); err != nil {
				return errors.Wrapf(err, "failed to create directory for mapped virtual disk %s", disk.ContainerPath)
			}

			if err := mount.MountWithTimedRetry(disk.ContainerPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// mountLayers mounts each device into a mountpoint, and then layers them into a
// union filesystem in the given order.
// These mountpoints are all stored under a directory reserved for the container
// with the given index.
func (c *gcsCore) mountLayers(index uint32, scratchMount *mountSpec, layers []*mountSpec) error {
	layerPrefix, scratchPath, upperdirPath, workdirPath, rootfsPath := c.getUnioningPaths(index)

	logrus.Infof("layerPrefix=%s", layerPrefix)
	logrus.Infof("scratchPath:%s", scratchPath)
	logrus.Infof("upperdirPath:%s", upperdirPath)
	logrus.Infof("workdirPath=%s", workdirPath)
	logrus.Infof("rootfsPath=%s", rootfsPath)

	// Mount the layer devices.
	layerPaths := make([]string, len(layers)+1)
	for i, layer := range layers {
		layerPath := filepath.Join(layerPrefix, strconv.Itoa(i))
		logrus.Infof("layerPath: %s", layerPath)
		if err := os.MkdirAll(layerPath, 0700); err != nil {
			return errors.Wrapf(err, "failed to create directory for layer %s", layerPath)
		}
		if err := layer.Mount(layerPath); err != nil {
			return errors.Wrapf(err, "failed to mount layer directory %s", layerPath)
		}
		layerPaths[i+1] = layerPath
	}
	// TODO: The base path code may be temporary until a more permanent DNS
	// solution is reached.
	// NOTE: This should probably still always be kept, because otherwise
	// mounting will fail when no layer devices are attached. There should
	// always be at least one layer, even if it's empty, to prevent this
	// from happening.
	layerPaths[0] = baseFilesPath

	// Mount the layers into a union filesystem.
	if err := os.MkdirAll(baseFilesPath, 0700); err != nil {
		return errors.Wrapf(err, "failed to create directory for base files %s", baseFilesPath)
	}
	if err := os.MkdirAll(scratchPath, 0755); err != nil {
		return errors.Wrapf(err, "failed to create directory for scratch space %s", scratchPath)
	}
	if scratchMount != nil {
		if err := scratchMount.Mount(scratchPath); err != nil {
			return errors.Wrapf(err, "failed to mount scratch directory %s", scratchPath)
		}
	} else {
		// NOTE: V1 has never supported a readonly overlay mount. It was "by
		// accident" always getting a writable overlay. Do nothing here if the
		// call does not have a scratch path.
	}
	return overlay.Mount(context.Background(), layerPaths, upperdirPath, workdirPath, rootfsPath, false)
}

// unmountLayers unmounts the union filesystem for the container with the given
// ID, as well as any devices whose mountpoints were layers in that filesystem.
func (c *gcsCore) unmountLayers(index uint32) error {
	layerPrefix, scratchPath, _, _, rootfsPath := c.getUnioningPaths(index)

	// clean up rootfsPath operations
	if err := storage.UnmountPath(context.Background(), rootfsPath, false); err != nil {
		return errors.Wrap(err, "failed to unmount root filesytem")
	}

	// clean up scratchPath operations
	if err := storage.UnmountPath(context.Background(), scratchPath, false); err != nil {
		return errors.Wrap(err, "failed to unmount scratch")
	}

	// Clean up layer path operations
	layerPaths, err := filepath.Glob(filepath.Join(layerPrefix, "*"))
	if err != nil {
		return errors.Wrap(err, "failed to get layer paths using Glob")
	}
	for _, layerPath := range layerPaths {
		if err := storage.UnmountPath(context.Background(), layerPath, false); err != nil {
			return errors.Wrap(err, "failed to unmount layer")
		}
	}

	return nil
}

// destroyContainerStorage removes any files the GCS stores on disk for the
// container with the given ID.
// These files include directories used for mountpoints in the union filesystem
// and config files.
func (c *gcsCore) destroyContainerStorage(index uint32) error {
	if err := os.RemoveAll(c.getContainerStoragePath(index)); err != nil {
		return errors.Wrapf(err, "failed to remove container storage path for container %s", c.getContainerIDFromIndex(index))
	}
	return nil
}

// writeConfigFile writes the given oci.Spec to disk so that it can be consumed
// by an OCI runtime.
func (c *gcsCore) writeConfigFile(index uint32, config *oci.Spec) error {
	if config == nil {
		return errors.New("failed to write init process config file, no options specified")
	}
	configPath := c.getConfigPath(index)
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return errors.Wrapf(err, "failed to create config file directory for container %s", c.getContainerIDFromIndex(index))
	}
	configFile, err := os.Create(configPath)
	if err != nil {
		return errors.Wrapf(err, "failed to create config file for container %s", c.getContainerIDFromIndex(index))
	}
	defer configFile.Close()
	writer := bufio.NewWriter(configFile)
	if err := json.NewEncoder(writer).Encode(config); err != nil {
		return errors.Wrapf(err, "failed to write contents of config file for container %s", c.getContainerIDFromIndex(index))
	}
	if err := writer.Flush(); err != nil {
		return errors.Wrapf(err, "failed to flush to config file for container %s", c.getContainerIDFromIndex(index))
	}
	return nil
}

// getContainerStoragePath returns the path where the GCS stores files on disk
// for the container with the given index.
func (c *gcsCore) getContainerStoragePath(index uint32) string {
	return filepath.Join(c.baseStoragePath, strconv.FormatUint(uint64(index), 10))
}

// getUnioningPaths returns paths that will be used in the union filesystem for
// the container with the given index.
func (c *gcsCore) getUnioningPaths(index uint32) (layerPrefix string, scratchPath string, upperdirPath string, workdirPath string, rootfsPath string) {
	mountPath := c.getContainerStoragePath(index)
	layerPrefix = mountPath
	scratchPath = filepath.Join(mountPath, "scratch")
	upperdirPath = filepath.Join(mountPath, "scratch", "upper")
	workdirPath = filepath.Join(mountPath, "scratch", "work")
	rootfsPath = filepath.Join(mountPath, "rootfs")
	return
}

// getConfigPath returns the path to the container's config file.
func (c *gcsCore) getConfigPath(index uint32) string {
	return filepath.Join(c.getContainerStoragePath(index), "config.json")
}

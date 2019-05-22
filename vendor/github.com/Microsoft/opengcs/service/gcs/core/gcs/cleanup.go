package gcs

import (
	"github.com/Microsoft/opengcs/internal/storage"
	"github.com/Microsoft/opengcs/service/gcs/runtime"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// CleanupContainer cleans up the state left behind by the container with the
// given ID.
// This function expects containerCacheMutex to be locked on entry.
func (c *gcsCore) cleanupContainer(containerEntry *containerCacheEntry) error {
	var errToReturn error
	if err := c.forceDeleteContainer(containerEntry.container); err != nil {
		logrus.Warn(err)
		if errToReturn == nil {
			errToReturn = err
		}
	}

	for _, disk := range containerEntry.MappedVirtualDisks {
		if !disk.AttachOnly {
			if err := storage.UnmountPath(disk.ContainerPath, false); err != nil {
				logrus.Warn(err)
				if errToReturn == nil {
					errToReturn = err
				}
			}
		}
	}
	for _, directory := range containerEntry.MappedDirectories {
		if err := storage.UnmountPath(directory.ContainerPath, false); err != nil {
			logrus.Warn(err)
			if errToReturn == nil {
				errToReturn = err
			}
		}
	}
	if err := c.unmountLayers(containerEntry.Index); err != nil {
		logrus.Warn(err)
		if errToReturn == nil {
			errToReturn = err
		}
	}

	// We only do cleanup if unmounting succeeds.
	if errToReturn == nil {
		if err := c.destroyContainerStorage(containerEntry.Index); err != nil {
			logrus.Warn(err)
			if errToReturn == nil {
				errToReturn = err
			}
		}
	} else {
		logrus.Warnf("Failed to unmount storage for container (%s). Will not delete!", containerEntry.ID)
		logrus.Warn(errToReturn)
	}

	return errToReturn
}

// forceDeleteContainer deletes the container, no matter its initial state.
func (c *gcsCore) forceDeleteContainer(container runtime.Container) error {
	exists, err := container.Exists()
	if err != nil {
		return err
	}
	if exists {
		state, err := container.GetState()
		if err != nil {
			return err
		}
		status := state.Status
		// If the container is paused, resume it.
		if status == "paused" {
			if err := container.Resume(); err != nil {
				return err
			}
			status = "running"
		}
		if status == "running" {
			if err := container.Kill(unix.SIGKILL); err != nil {
				return err
			}
			container.Wait()
		} else if status == "created" {
			// If we don't wait on a created container before deleting it, it
			// will become unblocked, and delete will fail.
			go container.Wait()
		}
		if err := container.Delete(); err != nil {
			return err
		}
	}
	return nil
}

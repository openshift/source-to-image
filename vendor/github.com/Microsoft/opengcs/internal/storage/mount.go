// +build linux

package storage

import (
	"os"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// Test dependencies
var (
	osStat      = os.Stat
	unixUnmount = unix.Unmount
	osRemoveAll = os.RemoveAll
)

// UnmountPath unmounts the target path if it exists and is a mount path. If
// removeTarget this will remove the previously mounted folder.
func UnmountPath(target string, removeTarget bool) (err error) {
	activity := "storage::UnmountPath"
	log := logrus.WithFields(logrus.Fields{
		"target": target,
		"remove": removeTarget,
	})
	log.Debug(activity + " - Begin Operation")
	defer func() {
		if err != nil {
			log.Data[logrus.ErrorKey] = err
			log.Error(activity + " - End Operation")
		} else {
			log.Debug(activity + " - End Operation")
		}
	}()

	if _, err := osStat(target); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to determine if path '%s' exists", target)
	}

	if err := unixUnmount(target, 0); err != nil {
		// If `Unmount` returns `EINVAL` it's not mounted. Just delete the
		// folder.
		if err != unix.EINVAL {
			return errors.Wrapf(err, "failed to unmount path '%s'", target)
		}
	}
	if removeTarget {
		return osRemoveAll(target)
	}
	return nil
}

// +build linux

package pmem

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// Test dependencies
var (
	osMkdirAll  = os.MkdirAll
	osRemoveAll = os.RemoveAll
	unixMount   = unix.Mount
)

// Mount mounts the pmem device at `/dev/pmem<device>` to `target`.
//
// `target` will be created. On mount failure the created `target` will be
// automatically cleaned up.
//
// Note: For now the platform only supports readonly pmem that is assumed to be
// `dax`, `ext4`.
func Mount(device uint32, target string) (err error) {
	activity := "pmem::Mount"
	log := logrus.WithFields(logrus.Fields{
		"device": device,
		"target": target,
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

	if err := osMkdirAll(target, 0700); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			osRemoveAll(target)
		}
	}()
	source := fmt.Sprintf("/dev/pmem%d", device)
	flags := uintptr(unix.MS_RDONLY)
	return unixMount(source, target, "ext4", flags, "noload,dax")
}

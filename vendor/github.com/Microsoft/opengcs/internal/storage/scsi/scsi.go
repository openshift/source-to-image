// +build linux

package scsi

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

const (
	// DeviceLookupTimeout is the amount of time before `ControllerLunToName`
	// gives up waiting for the `/dev/sd*` path to surface.
	DeviceLookupTimeout = 2 * time.Second
)

// Test dependencies
var (
	osMkdirAll  = os.MkdirAll
	osRemoveAll = os.RemoveAll
	unixMount   = unix.Mount

	// controllerLunToName is stubbed to make testing `Mount` easier.
	controllerLunToName = ControllerLunToName
)

// Mount creates a mount from the SCSI device on `controller` index `lun` to
// `target`
//
// `target` will be created. On mount failure the created `target` will be
// automatically cleaned up.
func Mount(controller, lun uint8, target string, readonly bool) (err error) {
	activity := "scsi::Mount"
	log := logrus.WithFields(logrus.Fields{
		"controller": controller,
		"lun":        lun,
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
	source, err := controllerLunToName(controller, lun)
	if err != nil {
		return err
	}
	var flags uintptr
	data := ""
	if readonly {
		flags |= unix.MS_RDONLY
		data = "noload"
	}

	start := time.Now()
	for {
		if err := unixMount(source, target, "ext4", flags, data); err != nil {
			// The `source` found by controllerLunToName can take some time
			// before its actually available under `/dev/sd*`. Retry while we
			// wait for `source` to show up.
			if err == unix.ENOENT && time.Since(start) < DeviceLookupTimeout {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			return err
		}
		break
	}
	return nil
}

// ControllerLunToName finds the `/dev/sd*` path to the SCSI device on
// `controller` index `lun`.
func ControllerLunToName(controller, lun uint8) (_ string, err error) {
	activity := "scsi::ControllerLunToName"
	log := logrus.WithFields(logrus.Fields{
		"controller": controller,
		"lun":        lun,
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

	scsiID := fmt.Sprintf("0:0:%d:%d", controller, lun)

	// Query for the device name up until the timeout.
	var deviceNames []os.FileInfo
	startTime := time.Now()
	for {
		// Devices matching the given SCSI code should each have a subdirectory
		// under /sys/bus/scsi/devices/<scsiID>/block.
		var err error
		deviceNames, err = ioutil.ReadDir(filepath.Join("/sys/bus/scsi/devices", scsiID, "block"))
		if err != nil {
			if time.Since(startTime) > DeviceLookupTimeout {
				return "", errors.Wrap(err, "failed to retrieve SCSI device names from filesystem")
			}
		} else {
			break
		}
		time.Sleep(time.Millisecond * 10)
	}

	if len(deviceNames) == 0 {
		return "", errors.Errorf("no matching device names found for SCSI ID \"%s\"", scsiID)
	}
	if len(deviceNames) > 1 {
		return "", errors.Errorf("more than one block device could match SCSI ID \"%s\"", scsiID)
	}
	return filepath.Join("/dev", deviceNames[0].Name()), nil
}

// UnplugDevice finds the SCSI device on `controller` index `lun` and issues a
// guest initiated unplug.
//
// If the device is not attached returns no error.
func UnplugDevice(controller, lun uint8) (err error) {
	activity := "scsi::UnplugDevice"
	log := logrus.WithFields(logrus.Fields{
		"controller": controller,
		"lun":        lun,
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

	scsiID := fmt.Sprintf("0:0:%d:%d", controller, lun)
	f, err := os.OpenFile(filepath.Join("/sys/bus/scsi/devices", scsiID, "delete"), os.O_WRONLY, 0644)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	if _, err := f.Write([]byte("1\n")); err != nil {
		return err
	}
	return nil
}

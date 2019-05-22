// +build linux

package network

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// GenerateResolvConfFile parses `dnsServerList` and `dnsSuffix` and writes the
// `nameserver` and `search` entries to `resolvPath`.
func GenerateResolvConfFile(resolvPath, dnsServerList, dnsSuffix string) (err error) {
	activity := "network::GenerateResolvConfFile"
	log := logrus.WithFields(logrus.Fields{
		"resolvPath":    resolvPath,
		"dnsServerList": dnsServerList,
		"dnsSuffix":     dnsSuffix,
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

	fileContents := ""

	split := func(r rune) bool {
		return r == ',' || r == ' '
	}

	nameservers := strings.FieldsFunc(dnsServerList, split)
	for i, server := range nameservers {
		// Limit number of nameservers to 3.
		if i >= 3 {
			break
		}

		fileContents += fmt.Sprintf("nameserver %s\n", server)
	}

	if dnsSuffix != "" {
		fileContents += fmt.Sprintf("search %s\n", dnsSuffix)
	}

	file, err := os.OpenFile(resolvPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return errors.Wrap(err, "failed to create resolv.conf")
	}
	defer file.Close()
	if _, err := io.WriteString(file, fileContents); err != nil {
		return errors.Wrapf(err, "failed to write to resolv.conf")
	}
	return nil
}

// InstanceIDToName converts from the given instance ID (a GUID generated on the
// Windows host) to its corresponding interface name (e.g. "eth0").
func InstanceIDToName(id string, wait bool) (_ string, err error) {
	activity := "network::InstanceIDToName"
	log := logrus.WithFields(logrus.Fields{
		"adapterInstanceID": id,
		"wait":              wait,
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

	const timeout = 2 * time.Second
	var deviceDirs []os.FileInfo
	start := time.Now()
	for {
		deviceDirs, err = ioutil.ReadDir(filepath.Join("/sys", "bus", "vmbus", "devices", id, "net"))
		if err != nil {
			if wait {
				if os.IsNotExist(errors.Cause(err)) {
					time.Sleep(10 * time.Millisecond)
					if time.Since(start) > timeout {
						return "", errors.Wrapf(err, "timed out waiting for net adapter after %d seconds", timeout)
					}
					continue
				}
			}
			return "", errors.Wrapf(err, "failed to read vmbus network device from /sys filesystem for adapter %s", id)
		}
		break
	}
	if len(deviceDirs) == 0 {
		return "", errors.Errorf("no interface name found for adapter %s", id)
	}
	if len(deviceDirs) > 1 {
		return "", errors.Errorf("multiple interface names found for adapter %s", id)
	}
	ifname := deviceDirs[0].Name()
	return ifname, nil
}

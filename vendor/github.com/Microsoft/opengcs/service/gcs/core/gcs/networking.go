package gcs

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Microsoft/opengcs/internal/network"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/runtime"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// configureAdapterInNamespace moves a given adapter into a network
// namespace and configures it there.
func (c *gcsCore) configureAdapterInNamespace(container runtime.Container, adapter prot.NetworkAdapter) error {
	interfaceName, err := network.InstanceIDToName(adapter.AdapterInstanceID, false)
	if err != nil {
		return err
	}
	nspid := container.Pid()
	cfg, err := json.Marshal(adapter)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal adapter struct to JSON for adapter %s", adapter.AdapterInstanceID)
	}

	out, err := exec.Command("netnscfg",
		"-if", interfaceName,
		"-nspid", fmt.Sprintf("%d", nspid),
		"-cfg", string(cfg)).CombinedOutput()
	if err != nil {
		logrus.Debugf("netnscfg failed: %s (%s)", out, err)
		return errors.Wrapf(err, "failed to configure network adapter %s: %s", adapter.AdapterInstanceID, out)
	}
	logrus.Debugf("netnscfg output:\n%s", out)

	// Handle resolve.conf
	// There is no need to create <baseFilesPath>/etc here as it
	// is created in CreateContainer().
	resolvPath := filepath.Join(baseFilesPath, "etc/resolv.conf")

	if adapter.NatEnabled {
		// Set the DNS configuration.
		if err := network.GenerateResolvConfFile(resolvPath, adapter.HostDNSServerList, adapter.HostDNSSuffix); err != nil {
			return errors.Wrapf(err, "failed to generate resolv.conf file for adapter %s", adapter.AdapterInstanceID)
		}
	} else {
		_, err := os.Stat(resolvPath)
		if err != nil {
			if os.IsNotExist(err) {
				if err := os.Link("/etc/resolv.conf", resolvPath); err != nil {
					return errors.Wrapf(err, "failed to link resolv.conf file for adapter %s", adapter.AdapterInstanceID)
				}
				return nil
			}
			return errors.Wrapf(err, "failed to check if resolv.conf path already exists for adapter %s", adapter.AdapterInstanceID)
		}
	}
	return nil
}

package gcs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Microsoft/opengcs/internal/network"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/runtime"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// configureAdapterInNamespace moves a given adapter into a network
// namespace and configures it there.
func (c *gcsCore) configureAdapterInNamespace(container runtime.Container, adapter prot.NetworkAdapter) error {
	interfaceName, err := network.InstanceIDToName(context.Background(), adapter.AdapterInstanceID, false)
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
		if err := c.generateResolvConfFile(resolvPath, adapter.HostDNSServerList, adapter.HostDNSSuffix); err != nil {
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

// generateResolvConfFile parses `dnsServerList` and `dnsSuffix` and writes the
// `nameserver` and `search` entries to `resolvPath`.
func (c *gcsCore) generateResolvConfFile(resolvPath, dnsServerList, dnsSuffix string) (err error) {
	logrus.WithFields(logrus.Fields{
		"resolvPath":    resolvPath,
		"dnsServerList": dnsServerList,
		"dnsSuffix":     dnsSuffix,
	}).Debug("generateResolvConfFile")

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

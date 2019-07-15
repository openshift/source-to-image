package hcsv2

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/Microsoft/opengcs/internal/network"
	"github.com/Microsoft/opengcs/internal/oc"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

func getStandaloneRootDir(id string) string {
	return filepath.Join("/tmp/gcs/s", id)
	return filepath.Join("/tmp/gcs/s", id)
}

func getStandaloneHostnamePath(id string) string {
	return filepath.Join(getStandaloneRootDir(id), "hostname")
}

func getStandaloneHostsPath(id string) string {
	return filepath.Join(getStandaloneRootDir(id), "hosts")
}

func getStandaloneResolvPath(id string) string {
	return filepath.Join(getStandaloneRootDir(id), "resolv.conf")
}

func setupStandaloneContainerSpec(ctx context.Context, id string, spec *oci.Spec) (err error) {
	ctx, span := trace.StartSpan(ctx, "hcsv2::setupStandaloneContainerSpec")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()
	span.AddAttributes(trace.StringAttribute("cid", id))

	// Generate the standalone root dir
	rootDir := getStandaloneRootDir(id)
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return errors.Wrapf(err, "failed to create container root directory %q", rootDir)
	}

	hostname := spec.Hostname
	if hostname == "" {
		var err error
		hostname, err = os.Hostname()
		if err != nil {
			return errors.Wrap(err, "failed to get hostname")
		}
	}

	// Write the hostname
	if !isInMounts("/etc/hostname", spec.Mounts) {
		standaloneHostnamePath := getStandaloneHostnamePath(id)
		if err := ioutil.WriteFile(standaloneHostnamePath, []byte(hostname+"\n"), 0644); err != nil {
			return errors.Wrapf(err, "failed to write hostname to %q", standaloneHostnamePath)
		}

		mt := oci.Mount{
			Destination: "/etc/hostname",
			Type:        "bind",
			Source:      getStandaloneHostnamePath(id),
			Options:     []string{"bind"},
		}
		if isRootReadonly(spec) {
			mt.Options = append(mt.Options, "ro")
		}
		spec.Mounts = append(spec.Mounts, mt)
	}

	// Write the hosts
	if !isInMounts("/etc/hosts", spec.Mounts) {
		standaloneHostsContent := network.GenerateEtcHostsContent(ctx, hostname)
		standaloneHostsPath := getStandaloneHostsPath(id)
		if err := ioutil.WriteFile(standaloneHostsPath, []byte(standaloneHostsContent), 0644); err != nil {
			return errors.Wrapf(err, "failed to write standalone hosts to %q", standaloneHostsPath)
		}

		mt := oci.Mount{
			Destination: "/etc/hosts",
			Type:        "bind",
			Source:      getStandaloneHostsPath(id),
			Options:     []string{"bind"},
		}
		if isRootReadonly(spec) {
			mt.Options = append(mt.Options, "ro")
		}
		spec.Mounts = append(spec.Mounts, mt)
	}

	// Write resolv.conf
	if !isInMounts("/etc/resolv.conf", spec.Mounts) {
		ns := getOrAddNetworkNamespace(getNetworkNamespaceID(spec))
		var searches, servers []string
		for _, n := range ns.Adapters() {
			servers = network.MergeValues(servers, strings.Split(n.DNSServerList, ","))
			servers = network.MergeValues(servers, strings.Split(n.DNSServerList, ","))
		}
		resolvContent, err := network.GenerateResolvConfContent(ctx, searches, servers, nil)
		if err != nil {
			return errors.Wrap(err, "failed to generate standalone resolv.conf content")
		}
		standaloneResolvPath := getStandaloneResolvPath(id)
		if err := ioutil.WriteFile(standaloneResolvPath, []byte(resolvContent), 0644); err != nil {
			return errors.Wrap(err, "failed to write standalone resolv.conf")
		}

		mt := oci.Mount{
			Destination: "/etc/resolv.conf",
			Type:        "bind",
			Source:      getStandaloneResolvPath(id),
			Options:     []string{"bind"},
		}
		if isRootReadonly(spec) {
			mt.Options = append(mt.Options, "ro")
		}
		spec.Mounts = append(spec.Mounts, mt)
	}

	// Clear the windows section as we dont want to forward to runc
	spec.Windows = nil

	return nil
}

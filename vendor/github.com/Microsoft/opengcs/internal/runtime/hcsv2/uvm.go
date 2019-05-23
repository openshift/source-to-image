// +build linux

package hcsv2

import (
	"bufio"
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/Microsoft/opengcs/internal/network"
	"github.com/Microsoft/opengcs/internal/storage"
	"github.com/Microsoft/opengcs/internal/storage/overlay"
	"github.com/Microsoft/opengcs/internal/storage/plan9"
	"github.com/Microsoft/opengcs/internal/storage/pmem"
	"github.com/Microsoft/opengcs/internal/storage/scsi"
	"github.com/Microsoft/opengcs/service/gcs/gcserr"
	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcs/runtime"
	"github.com/Microsoft/opengcs/service/gcs/transport"
	"github.com/opencontainers/runc/libcontainer/devices"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// UVMContainerID is the ContainerID that will be sent on any prot.MessageBase
// for V2 where the specific message is targeted at the UVM itself.
const UVMContainerID = "00000000-0000-0000-0000-000000000000"

// Host is the structure tracking all UVM host state including all containers
// and processes.
type Host struct {
	containersMutex sync.Mutex
	containers      map[string]*Container

	// Rtime is the Runtime interface used by the GCS core.
	rtime runtime.Runtime
	vsock transport.Transport

	// netNamespaces maps `NamespaceID` to the namespace opts
	netNamespaces map[string]*netNSOpts
	// networkNSToContainer is a map from `NamespaceID` to sandbox
	// `ContainerID`. If the map entry does not exist then the adapter is cached
	// in `netNamespaces` for addition when the sandbox is eventually created.
	networkNSToContainer sync.Map
}

func NewHost(rtime runtime.Runtime, vsock transport.Transport) *Host {
	return &Host{
		containers:    make(map[string]*Container),
		rtime:         rtime,
		vsock:         vsock,
		netNamespaces: make(map[string]*netNSOpts),
	}
}

func (h *Host) getContainerLocked(id string) (*Container, error) {
	if c, ok := h.containers[id]; !ok {
		return nil, gcserr.NewHresultError(gcserr.HrVmcomputeSystemNotFound)
	} else {
		return c, nil
	}
}

func (h *Host) GetContainer(id string) (*Container, error) {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	return h.getContainerLocked(id)
}

type netNSOpts struct {
	Adapters   []*prot.NetworkAdapterV2
	ResolvPath string
}

func (h *Host) CreateContainer(id string, settings *prot.VMHostedContainerSettingsV2) (*Container, error) {
	h.containersMutex.Lock()
	defer h.containersMutex.Unlock()

	if _, ok := h.containers[id]; ok {
		return nil, gcserr.NewHresultError(gcserr.HrVmcomputeSystemAlreadyExists)
	}

	isSandboxOrStandalone := false
	if criType, ok := settings.OCISpecification.Annotations["io.kubernetes.cri.container-type"]; ok {
		// If the annotation is present it must be "sandbox"
		isSandboxOrStandalone = criType == "sandbox"
	} else {
		// No annotation declares a standalone LCOW
		isSandboxOrStandalone = true
	}

	// Check if we have a network namespace and if so generate the resolv.conf
	// so we can bindmount it into the container
	var networkNamespace string
	if settings.OCISpecification.Windows != nil &&
		settings.OCISpecification.Windows.Network != nil &&
		settings.OCISpecification.Windows.Network.NetworkNamespace != "" {
		networkNamespace = strings.ToLower(settings.OCISpecification.Windows.Network.NetworkNamespace)
	}
	if networkNamespace != "" {
		if isSandboxOrStandalone {
			// We have a network namespace. Generate the resolv.conf and add the bind mount.
			netopts := h.netNamespaces[networkNamespace]
			if netopts == nil || len(netopts.Adapters) == 0 {
				logrus.WithFields(logrus.Fields{
					"cid":   id,
					"netNS": networkNamespace,
				}).Warn("opengcs::CreateContainer - No adapters in namespace for sandbox")
			} else {
				logrus.WithFields(logrus.Fields{
					"cid":   id,
					"netNS": networkNamespace,
				}).Info("opengcs::CreateContainer - Generate sandbox resolv.conf")

				// TODO: Can we ever have more than one nic here at start?
				td, err := ioutil.TempDir("", "")
				if err != nil {
					return nil, errors.Wrap(err, "failed to create tmp directory")
				}
				resolvPath := filepath.Join(td, "resolv.conf")
				adp := netopts.Adapters[0]
				err = network.GenerateResolvConfFile(resolvPath, adp.DNSServerList, adp.DNSSuffix)
				if err != nil {
					return nil, err
				}
				// Store the resolve path for all workload containers
				netopts.ResolvPath = resolvPath
			}
		}
		if netopts, ok := h.netNamespaces[networkNamespace]; ok && netopts.ResolvPath != "" {
			logrus.WithFields(logrus.Fields{
				"cid":        id,
				"netNS":      networkNamespace,
				"resolvPath": netopts.ResolvPath,
			}).Info("opengcs::CreateContainer - Adding /etc/resolv.conf mount")

			settings.OCISpecification.Mounts = append(settings.OCISpecification.Mounts, oci.Mount{
				Destination: "/etc/resolv.conf",
				Type:        "bind",
				Source:      netopts.ResolvPath,
				Options:     []string{"bind", "ro"},
			})
		}
	}

	// Clear the windows section of the config
	settings.OCISpecification.Windows = nil

	// Check if we need to do any capability/device mappings
	if settings.OCISpecification.Annotations["io.microsoft.virtualmachine.lcow.privileged"] == "true" {
		logrus.WithFields(logrus.Fields{
			"cid": id,
		}).Debugf("opengcs::CreateContainer - 'io.microsoft.virtualmachine.lcow.privileged' set for privileged container")

		// Add all host devices
		hostDevices, err := devices.HostDevices()
		if err != nil {
			return nil, err
		}
		for _, hostDevice := range hostDevices {
			rd := oci.LinuxDevice{
				Path:  hostDevice.Path,
				Type:  string(hostDevice.Type),
				Major: hostDevice.Major,
				Minor: hostDevice.Minor,
				UID:   &hostDevice.Uid,
				GID:   &hostDevice.Gid,
			}
			if hostDevice.Major == 0 && hostDevice.Minor == 0 {
				// Invalid device, most likely a symbolic link, skip it.
				continue
			}
			found := false
			for i, dev := range settings.OCISpecification.Linux.Devices {
				if dev.Path == rd.Path {
					found = true
					settings.OCISpecification.Linux.Devices[i] = rd
					break
				}
				if dev.Type == rd.Type && dev.Major == rd.Major && dev.Minor == rd.Minor {
					logrus.WithFields(logrus.Fields{
						"cid": id,
					}).Warnf("opengcs::CreateContainer - The same type '%s', major '%d' and minor '%d', should not be used for multiple devices.", dev.Type, dev.Major, dev.Minor)
				}
			}
			if !found {
				settings.OCISpecification.Linux.Devices = append(settings.OCISpecification.Linux.Devices, rd)
			}
		}

		// Set the cgroup access
		settings.OCISpecification.Linux.Resources.Devices = []oci.LinuxDeviceCgroup{
			{
				Allow:  true,
				Access: "rwm",
			},
		}
	}

	// Container doesnt exit. Create it here
	// Create the BundlePath
	if err := os.MkdirAll(settings.OCIBundlePath, 0700); err != nil {
		return nil, errors.Wrapf(err, "failed to create OCIBundlePath: '%s'", settings.OCIBundlePath)
	}
	configFile := path.Join(settings.OCIBundlePath, "config.json")
	f, err := os.Create(configFile)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create config.json at: '%s'", configFile)
	}
	defer f.Close()
	writer := bufio.NewWriter(f)
	if err := json.NewEncoder(writer).Encode(settings.OCISpecification); err != nil {
		return nil, errors.Wrapf(err, "failed to write OCISpecification to config.json at: '%s'", configFile)
	}
	if err := writer.Flush(); err != nil {
		return nil, errors.Wrapf(err, "failed to flush writer for config.json at: '%s'", configFile)
	}

	con, err := h.rtime.CreateContainer(id, settings.OCIBundlePath, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create container")
	}

	c := &Container{
		id:        id,
		vsock:     h.vsock,
		spec:      settings.OCISpecification,
		container: con,
		exitType:  prot.NtUnexpectedExit,
		processes: make(map[uint32]*Process),
	}
	// Add the WG count for the init process
	c.processesWg.Add(1)
	c.initProcess = newProcess(c, settings.OCISpecification.Process, con.(runtime.Process), uint32(c.container.Pid()), true)

	// For the sandbox move all adapters into the network namespace
	if isSandboxOrStandalone && networkNamespace != "" {
		if netopts, ok := h.netNamespaces[networkNamespace]; ok {
			for _, a := range netopts.Adapters {
				err = c.AddNetworkAdapter(a)
				if err != nil {
					return nil, err
				}
			}
		}
		// Add a link for all HotAdd/Remove from NS id to this container.
		h.networkNSToContainer.Store(networkNamespace, id)
	}

	h.containers[id] = c
	return c, nil
}

func (h *Host) ModifyHostSettings(settings *prot.ModifySettingRequest) error {
	type modifyFunc func(interface{}) error

	requestTypeFn := func(req prot.ModifyRequestType, setting interface{}, add, remove, update modifyFunc) error {
		switch req {
		case prot.MreqtAdd:
			if add != nil {
				return add(setting)
			}
			break
		case prot.MreqtRemove:
			if remove != nil {
				return remove(setting)
			}
			break
		case prot.MreqtUpdate:
			if update != nil {
				return update(setting)
			}
			break
		}

		return errors.Errorf("the RequestType \"%s\" is not supported", req)
	}

	var add modifyFunc
	var remove modifyFunc
	var update modifyFunc

	switch settings.ResourceType {
	case prot.MrtMappedVirtualDisk:
		add = func(setting interface{}) error {
			mvd := setting.(*prot.MappedVirtualDiskV2)
			if mvd.MountPath != "" {
				return scsi.Mount(mvd.Controller, mvd.Lun, mvd.MountPath, mvd.ReadOnly)
			}
			return nil
		}
		remove = func(setting interface{}) error {
			mvd := setting.(*prot.MappedVirtualDiskV2)
			if mvd.MountPath != "" {
				if err := storage.UnmountPath(mvd.MountPath, true); err != nil {
					return errors.Wrapf(err, "failed to hot remove MappedVirtualDiskV2 path: '%s'", mvd.MountPath)
				}
			}
			return scsi.UnplugDevice(mvd.Controller, mvd.Lun)
		}
	case prot.MrtMappedDirectory:
		add = func(setting interface{}) error {
			md := setting.(*prot.MappedDirectoryV2)
			return plan9.Mount(h.vsock, md.MountPath, md.ShareName, md.Port, md.ReadOnly)
		}
		remove = func(setting interface{}) error {
			md := setting.(*prot.MappedDirectoryV2)
			return storage.UnmountPath(md.MountPath, true)
		}
	case prot.MrtVPMemDevice:
		add = func(setting interface{}) error {
			vpd := setting.(*prot.MappedVPMemDeviceV2)
			return pmem.Mount(vpd.DeviceNumber, vpd.MountPath)
		}
		remove = func(setting interface{}) error {
			vpd := setting.(*prot.MappedVPMemDeviceV2)
			return storage.UnmountPath(vpd.MountPath, true)
		}
	case prot.MrtCombinedLayers:
		add = func(setting interface{}) error {
			cl := setting.(*prot.CombinedLayersV2)
			layerPaths := make([]string, len(cl.Layers))
			for i, layer := range cl.Layers {
				layerPaths[i] = layer.Path
			}

			var upperdirPath string
			var workdirPath string
			readonly := false
			if cl.ScratchPath == "" {
				// The user did not pass a scratch path. Mount overlay as readonly.
				readonly = true
			} else {
				upperdirPath = filepath.Join(cl.ScratchPath, "upper")
				workdirPath = filepath.Join(cl.ScratchPath, "work")
			}

			return overlay.Mount(layerPaths, upperdirPath, workdirPath, cl.ContainerRootPath, readonly)
		}
		remove = func(setting interface{}) error {
			cl := setting.(*prot.CombinedLayersV2)
			return storage.UnmountPath(cl.ContainerRootPath, true)
		}
	case prot.MrtNetwork:
		add = func(setting interface{}) error {
			na := setting.(*prot.NetworkAdapterV2)
			na.ID = strings.ToLower(na.ID)
			na.NamespaceID = strings.ToLower(na.NamespaceID)
			if cidraw, ok := h.networkNSToContainer.Load(na.NamespaceID); ok {
				// The container has already been created. Get it and add this
				// adapter in real time.
				c, err := h.GetContainer(cidraw.(string))
				if err != nil {
					return err
				}
				return c.AddNetworkAdapter(na)
			}
			netopts, ok := h.netNamespaces[na.NamespaceID]
			if !ok {
				netopts = &netNSOpts{}
				h.netNamespaces[na.NamespaceID] = netopts
			}
			netopts.Adapters = append(netopts.Adapters, na)
			return nil
		}
		remove = func(setting interface{}) error {
			na := setting.(*prot.NetworkAdapterV2)
			na.ID = strings.ToLower(na.ID)
			na.NamespaceID = strings.ToLower(na.NamespaceID)
			if cidraw, ok := h.networkNSToContainer.Load(na.NamespaceID); ok {
				// The container was previously created or is still. Remove the
				// network. If the container is not found we just remove the
				// namespace reference.
				if c, err := h.GetContainer(cidraw.(string)); err == nil {
					return c.RemoveNetworkAdapter(na.ID)
				}
			} else {
				if netopts, ok := h.netNamespaces[na.NamespaceID]; ok {
					var i int
					var a *prot.NetworkAdapterV2
					for i, a = range netopts.Adapters {
						if na.ID == a.ID {
							break
						}
					}
					if a != nil {
						netopts.Adapters = append(netopts.Adapters[:i], netopts.Adapters[i+1:]...)
					}
				}
			}
			return nil
		}
	default:
		return errors.Errorf("the resource type \"%s\" is not supported", settings.ResourceType)
	}

	if err := requestTypeFn(settings.RequestType, settings.Settings, add, remove, update); err != nil {
		return errors.Wrapf(err, "Failed to modify ResourceType: \"%s\"", settings.ResourceType)
	}
	return nil
}

// Shutdown terminates this UVM. This is a destructive call and will destroy all
// state that has not been cleaned before calling this function.
func (h *Host) Shutdown() {
	syscall.Reboot(syscall.LINUX_REBOOT_CMD_POWER_OFF)
}

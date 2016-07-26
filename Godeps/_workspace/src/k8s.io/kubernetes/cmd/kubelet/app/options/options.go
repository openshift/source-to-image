/*
Copyright 2015 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package options contains all of the primary arguments for a kubelet.
package options

import (
	_ "net/http/pprof"
	"runtime"
	"time"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/apis/componentconfig"
	"k8s.io/kubernetes/pkg/kubelet/qos"
	"k8s.io/kubernetes/pkg/kubelet/rkt"
	kubetypes "k8s.io/kubernetes/pkg/kubelet/types"
	"k8s.io/kubernetes/pkg/master/ports"
	"k8s.io/kubernetes/pkg/util"
	"k8s.io/kubernetes/pkg/util/config"

	"github.com/spf13/pflag"
)

const (
	defaultRootDir             = "/var/lib/kubelet"
	experimentalFlannelOverlay = false

	defaultPodInfraContainerImageName    = "gcr.io/google_containers/pause"
	defaultPodInfraContainerImageVersion = "3.0"
)

// Returns the arch-specific pause image that kubelet should use as the default
func GetDefaultPodInfraContainerImage() string {
	return defaultPodInfraContainerImageName + "-" + runtime.GOARCH + ":" + defaultPodInfraContainerImageVersion
}

// KubeletServer encapsulates all of the parameters necessary for starting up
// a kubelet. These can either be set via command line or directly.
type KubeletServer struct {
	componentconfig.KubeletConfiguration

	AuthPath      util.StringFlag // Deprecated -- use KubeConfig instead
	KubeConfig    util.StringFlag
	APIServerList []string

	RunOnce bool

	// Insert a probability of random errors during calls to the master.
	ChaosChance float64
	// Crash immediately, rather than eating panics.
	ReallyCrashForTesting bool
	SystemReserved        config.ConfigurationMap
	KubeReserved          config.ConfigurationMap
}

// NewKubeletServer will create a new KubeletServer with default values.
func NewKubeletServer() *KubeletServer {
	return &KubeletServer{
		AuthPath:   util.NewStringFlag("/var/lib/kubelet/kubernetes_auth"), // deprecated
		KubeConfig: util.NewStringFlag("/var/lib/kubelet/kubeconfig"),

		SystemReserved: make(config.ConfigurationMap),
		KubeReserved:   make(config.ConfigurationMap),
		KubeletConfiguration: componentconfig.KubeletConfiguration{
			Address:                     "0.0.0.0",
			CAdvisorPort:                4194,
			VolumeStatsAggPeriod:        unversioned.Duration{Duration: time.Minute},
			CertDirectory:               "/var/run/kubernetes",
			CgroupRoot:                  "",
			ConfigureCBR0:               false,
			ContainerRuntime:            "docker",
			CPUCFSQuota:                 true,
			DockerExecHandlerName:       "native",
			EventBurst:                  10,
			EventRecordQPS:              5.0,
			EnableCustomMetrics:         false,
			EnableDebuggingHandlers:     true,
			EnableServer:                true,
			FileCheckFrequency:          unversioned.Duration{Duration: 20 * time.Second},
			HealthzBindAddress:          "127.0.0.1",
			HealthzPort:                 10248,
			HostNetworkSources:          kubetypes.AllSource,
			HostPIDSources:              kubetypes.AllSource,
			HostIPCSources:              kubetypes.AllSource,
			HTTPCheckFrequency:          unversioned.Duration{Duration: 20 * time.Second},
			ImageMinimumGCAge:           unversioned.Duration{Duration: 2 * time.Minute},
			ImageGCHighThresholdPercent: 90,
			ImageGCLowThresholdPercent:  80,
			LowDiskSpaceThresholdMB:     256,
			MasterServiceNamespace:      api.NamespaceDefault,
			MaxContainerCount:           240,
			MaxPerPodContainerCount:     2,
			MaxOpenFiles:                1000000,
			MaxPods:                     110,
			MinimumGCAge:                unversioned.Duration{Duration: 1 * time.Minute},
			NetworkPluginDir:            "/usr/libexec/kubernetes/kubelet-plugins/net/exec/",
			NetworkPluginName:           "",
			NonMasqueradeCIDR:           "10.0.0.0/8",
			VolumePluginDir:             "/usr/libexec/kubernetes/kubelet-plugins/volume/exec/",
			NodeStatusUpdateFrequency:   unversioned.Duration{Duration: 10 * time.Second},
			NodeLabels:                  make(map[string]string),
			OOMScoreAdj:                 int32(qos.KubeletOOMScoreAdj),
			LockFilePath:                "",
			PodInfraContainerImage:      GetDefaultPodInfraContainerImage(),
			Port:                           ports.KubeletPort,
			ReadOnlyPort:                   ports.KubeletReadOnlyPort,
			RegisterNode:                   true, // will be ignored if no apiserver is configured
			RegisterSchedulable:            true,
			RegistryBurst:                  10,
			RegistryPullQPS:                5.0,
			KubeletCgroups:                 "",
			ResolverConfig:                 kubetypes.ResolvConfDefault,
			RktPath:                        "",
			RktAPIEndpoint:                 rkt.DefaultRktAPIServiceEndpoint,
			RktStage1Image:                 "",
			RootDirectory:                  defaultRootDir,
			RuntimeCgroups:                 "",
			SerializeImagePulls:            true,
			StreamingConnectionIdleTimeout: unversioned.Duration{Duration: 4 * time.Hour},
			SyncFrequency:                  unversioned.Duration{Duration: 1 * time.Minute},
			SystemCgroups:                  "",
			ReconcileCIDR:                  true,
			KubeAPIQPS:                     5.0,
			KubeAPIBurst:                   10,
			ExperimentalFlannelOverlay:     experimentalFlannelOverlay,
			OutOfDiskTransitionFrequency:   unversioned.Duration{Duration: 5 * time.Minute},
			HairpinMode:                    componentconfig.PromiscuousBridge,
			BabysitDaemons:                 false,
		},
	}
}

// AddFlags adds flags for a specific KubeletServer to the specified FlagSet
func (s *KubeletServer) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&s.Config, "config", s.Config, "Path to the config file or directory of files")
	fs.DurationVar(&s.SyncFrequency.Duration, "sync-frequency", s.SyncFrequency.Duration, "Max period between synchronizing running containers and config")
	fs.DurationVar(&s.FileCheckFrequency.Duration, "file-check-frequency", s.FileCheckFrequency.Duration, "Duration between checking config files for new data")
	fs.DurationVar(&s.HTTPCheckFrequency.Duration, "http-check-frequency", s.HTTPCheckFrequency.Duration, "Duration between checking http for new data")
	fs.StringVar(&s.ManifestURL, "manifest-url", s.ManifestURL, "URL for accessing the container manifest")
	fs.StringVar(&s.ManifestURLHeader, "manifest-url-header", s.ManifestURLHeader, "HTTP header to use when accessing the manifest URL, with the key separated from the value with a ':', as in 'key:value'")
	fs.BoolVar(&s.EnableServer, "enable-server", s.EnableServer, "Enable the Kubelet's server")
	fs.Var(componentconfig.IPVar{Val: &s.Address}, "address", "The IP address for the Kubelet to serve on (set to 0.0.0.0 for all interfaces)")
	fs.UintVar(&s.Port, "port", s.Port, "The port for the Kubelet to serve on.")
	fs.UintVar(&s.ReadOnlyPort, "read-only-port", s.ReadOnlyPort, "The read-only port for the Kubelet to serve on with no authentication/authorization (set to 0 to disable)")
	fs.StringVar(&s.TLSCertFile, "tls-cert-file", s.TLSCertFile, ""+
		"File containing x509 Certificate for HTTPS.  (CA cert, if any, concatenated after server cert). "+
		"If --tls-cert-file and --tls-private-key-file are not provided, a self-signed certificate and key "+
		"are generated for the public address and saved to the directory passed to --cert-dir.")
	fs.StringVar(&s.TLSPrivateKeyFile, "tls-private-key-file", s.TLSPrivateKeyFile, "File containing x509 private key matching --tls-cert-file.")
	fs.StringVar(&s.CertDirectory, "cert-dir", s.CertDirectory, "The directory where the TLS certs are located (by default /var/run/kubernetes). "+
		"If --tls-cert-file and --tls-private-key-file are provided, this flag will be ignored.")
	fs.StringVar(&s.HostnameOverride, "hostname-override", s.HostnameOverride, "If non-empty, will use this string as identification instead of the actual hostname.")
	fs.StringVar(&s.PodInfraContainerImage, "pod-infra-container-image", s.PodInfraContainerImage, "The image whose network/ipc namespaces containers in each pod will use.")
	fs.StringVar(&s.DockerEndpoint, "docker-endpoint", s.DockerEndpoint, "If non-empty, use this for the docker endpoint to communicate with")
	fs.StringVar(&s.RootDirectory, "root-dir", s.RootDirectory, "Directory path for managing kubelet files (volume mounts,etc).")
	fs.BoolVar(&s.AllowPrivileged, "allow-privileged", s.AllowPrivileged, "If true, allow containers to request privileged mode. [default=false]")
	fs.StringVar(&s.HostNetworkSources, "host-network-sources", s.HostNetworkSources, "Comma-separated list of sources from which the Kubelet allows pods to use of host network. [default=\"*\"]")
	fs.StringVar(&s.HostPIDSources, "host-pid-sources", s.HostPIDSources, "Comma-separated list of sources from which the Kubelet allows pods to use the host pid namespace. [default=\"*\"]")
	fs.StringVar(&s.HostIPCSources, "host-ipc-sources", s.HostIPCSources, "Comma-separated list of sources from which the Kubelet allows pods to use the host ipc namespace. [default=\"*\"]")
	fs.Float64Var(&s.RegistryPullQPS, "registry-qps", s.RegistryPullQPS, "If > 0, limit registry pull QPS to this value.  If 0, unlimited. [default=5.0]")
	fs.Int32Var(&s.RegistryBurst, "registry-burst", s.RegistryBurst, "Maximum size of a bursty pulls, temporarily allows pulls to burst to this number, while still not exceeding registry-qps.  Only used if --registry-qps > 0")
	fs.Float32Var(&s.EventRecordQPS, "event-qps", s.EventRecordQPS, "If > 0, limit event creations per second to this value. If 0, unlimited.")
	fs.Int32Var(&s.EventBurst, "event-burst", s.EventBurst, "Maximum size of a bursty event records, temporarily allows event records to burst to this number, while still not exceeding event-qps. Only used if --event-qps > 0")
	fs.BoolVar(&s.RunOnce, "runonce", s.RunOnce, "If true, exit after spawning pods from local manifests or remote urls. Exclusive with --api-servers, and --enable-server")
	fs.BoolVar(&s.EnableDebuggingHandlers, "enable-debugging-handlers", s.EnableDebuggingHandlers, "Enables server endpoints for log collection and local running of containers and commands")
	fs.DurationVar(&s.MinimumGCAge.Duration, "minimum-container-ttl-duration", s.MinimumGCAge.Duration, "Minimum age for a finished container before it is garbage collected.  Examples: '300ms', '10s' or '2h45m'")
	fs.Int32Var(&s.MaxPerPodContainerCount, "maximum-dead-containers-per-container", s.MaxPerPodContainerCount, "Maximum number of old instances to retain per container.  Each container takes up some disk space.  Default: 2.")
	fs.Int32Var(&s.MaxContainerCount, "maximum-dead-containers", s.MaxContainerCount, "Maximum number of old instances of containers to retain globally.  Each container takes up some disk space.  Default: 100.")
	fs.Var(&s.AuthPath, "auth-path", "Path to .kubernetes_auth file, specifying how to authenticate to API server.")
	fs.MarkDeprecated("auth-path", "will be removed in a future version")
	fs.Var(&s.KubeConfig, "kubeconfig", "Path to a kubeconfig file, specifying how to authenticate to API server (the master location is set by the api-servers flag).")
	fs.UintVar(&s.CAdvisorPort, "cadvisor-port", s.CAdvisorPort, "The port of the localhost cAdvisor endpoint")
	fs.Int32Var(&s.HealthzPort, "healthz-port", s.HealthzPort, "The port of the localhost healthz endpoint")
	fs.Var(componentconfig.IPVar{Val: &s.HealthzBindAddress}, "healthz-bind-address", "The IP address for the healthz server to serve on, defaulting to 127.0.0.1 (set to 0.0.0.0 for all interfaces)")
	fs.Int32Var(&s.OOMScoreAdj, "oom-score-adj", s.OOMScoreAdj, "The oom-score-adj value for kubelet process. Values must be within the range [-1000, 1000]")
	fs.StringSliceVar(&s.APIServerList, "api-servers", []string{}, "List of Kubernetes API servers for publishing events, and reading pods and services. (ip:port), comma separated.")
	fs.BoolVar(&s.RegisterNode, "register-node", s.RegisterNode, "Register the node with the apiserver (defaults to true if --api-servers is set)")
	fs.StringVar(&s.ClusterDomain, "cluster-domain", s.ClusterDomain, "Domain for this cluster.  If set, kubelet will configure all containers to search this domain in addition to the host's search domains")
	fs.StringVar(&s.MasterServiceNamespace, "master-service-namespace", s.MasterServiceNamespace, "The namespace from which the kubernetes master services should be injected into pods")
	fs.StringVar(&s.ClusterDNS, "cluster-dns", s.ClusterDNS, "IP address for a cluster DNS server.  This value is used for containers' DNS server in case of Pods with \"dnsPolicy=ClusterFirst\"")
	fs.DurationVar(&s.StreamingConnectionIdleTimeout.Duration, "streaming-connection-idle-timeout", s.StreamingConnectionIdleTimeout.Duration, "Maximum time a streaming connection can be idle before the connection is automatically closed. 0 indicates no timeout. Example: '5m'")
	fs.DurationVar(&s.NodeStatusUpdateFrequency.Duration, "node-status-update-frequency", s.NodeStatusUpdateFrequency.Duration, "Specifies how often kubelet posts node status to master. Note: be cautious when changing the constant, it must work with nodeMonitorGracePeriod in nodecontroller. Default: 10s")
	bindableNodeLabels := config.ConfigurationMap(s.NodeLabels)
	fs.Var(&bindableNodeLabels, "node-labels", "<Warning: Alpha feature> Labels to add when registering the node in the cluster.  Labels must be key=value pairs separated by ','.")
	fs.DurationVar(&s.ImageMinimumGCAge.Duration, "minimum-image-ttl-duration", s.ImageMinimumGCAge.Duration, "Minimum age for a unused image before it is garbage collected.  Examples: '300ms', '10s' or '2h45m'. Default: '2m'")
	fs.Int32Var(&s.ImageGCHighThresholdPercent, "image-gc-high-threshold", s.ImageGCHighThresholdPercent, "The percent of disk usage after which image garbage collection is always run. Default: 90%")
	fs.Int32Var(&s.ImageGCLowThresholdPercent, "image-gc-low-threshold", s.ImageGCLowThresholdPercent, "The percent of disk usage before which image garbage collection is never run. Lowest disk usage to garbage collect to. Default: 80%")
	fs.Int32Var(&s.LowDiskSpaceThresholdMB, "low-diskspace-threshold-mb", s.LowDiskSpaceThresholdMB, "The absolute free disk space, in MB, to maintain. When disk space falls below this threshold, new pods would be rejected. Default: 256")
	fs.DurationVar(&s.VolumeStatsAggPeriod.Duration, "volume-stats-agg-period", s.VolumeStatsAggPeriod.Duration, "Specifies interval for kubelet to calculate and cache the volume disk usage for all pods and volumes.  To disable volume calculations, set to 0.  Default: '1m'")
	fs.StringVar(&s.NetworkPluginName, "network-plugin", s.NetworkPluginName, "<Warning: Alpha feature> The name of the network plugin to be invoked for various events in kubelet/pod lifecycle")
	fs.StringVar(&s.NetworkPluginDir, "network-plugin-dir", s.NetworkPluginDir, "<Warning: Alpha feature> The full path of the directory in which to search for network plugins")
	fs.StringVar(&s.VolumePluginDir, "volume-plugin-dir", s.VolumePluginDir, "<Warning: Alpha feature> The full path of the directory in which to search for additional third party volume plugins")
	fs.StringVar(&s.CloudProvider, "cloud-provider", s.CloudProvider, "The provider for cloud services.  Empty string for no provider.")
	fs.StringVar(&s.CloudConfigFile, "cloud-config", s.CloudConfigFile, "The path to the cloud provider configuration file.  Empty string for no configuration file.")

	fs.StringVar(&s.KubeletCgroups, "resource-container", s.KubeletCgroups, "Optional absolute name of the resource-only container to create and run the Kubelet in.")
	fs.MarkDeprecated("resource-container", "Use --kubelet-cgroups instead. Will be removed in a future version.")
	fs.StringVar(&s.KubeletCgroups, "kubelet-cgroups", s.KubeletCgroups, "Optional absolute name of cgroups to create and run the Kubelet in.")

	fs.StringVar(&s.SystemCgroups, "system-container", s.SystemCgroups, "Optional resource-only container in which to place all non-kernel processes that are not already in a container. Empty for no container. Rolling back the flag requires a reboot. (Default: \"\").")
	fs.MarkDeprecated("system-container", "Use --system-cgroups instead. Will be removed in a future version.")
	fs.StringVar(&s.SystemCgroups, "system-cgroups", s.SystemCgroups, "Optional absolute name of cgroups in which to place all non-kernel processes that are not already inside a cgroup under `/`. Empty for no container. Rolling back the flag requires a reboot. (Default: \"\").")

	fs.StringVar(&s.CgroupRoot, "cgroup-root", s.CgroupRoot, "Optional root cgroup to use for pods. This is handled by the container runtime on a best effort basis. Default: '', which means use the container runtime default.")
	fs.StringVar(&s.ContainerRuntime, "container-runtime", s.ContainerRuntime, "The container runtime to use. Possible values: 'docker', 'rkt'. Default: 'docker'.")
	fs.StringVar(&s.LockFilePath, "lock-file", s.LockFilePath, "<Warning: Alpha feature> The path to file for kubelet to use as a lock file.")
	fs.StringVar(&s.RktPath, "rkt-path", s.RktPath, "Path of rkt binary. Leave empty to use the first rkt in $PATH.  Only used if --container-runtime='rkt'.")
	fs.StringVar(&s.RktAPIEndpoint, "rkt-api-endpoint", s.RktAPIEndpoint, "The endpoint of the rkt API service to communicate with. Only used if --container-runtime='rkt'.")
	fs.StringVar(&s.RktStage1Image, "rkt-stage1-image", s.RktStage1Image, "image to use as stage1. Local paths and http/https URLs are supported. If empty, the 'stage1.aci' in the same directory as '--rkt-path' will be used.")
	fs.MarkDeprecated("rkt-stage1-image", "Will be removed in a future version. The default stage1 image will be specified by the rkt configurations, see https://github.com/coreos/rkt/blob/master/Documentation/configuration.md for more details.")
	fs.BoolVar(&s.ConfigureCBR0, "configure-cbr0", s.ConfigureCBR0, "If true, kubelet will configure cbr0 based on Node.Spec.PodCIDR.")
	fs.StringVar(&s.HairpinMode, "hairpin-mode", s.HairpinMode, "How should the kubelet setup hairpin NAT. This allows endpoints of a Service to loadbalance back to themselves if they should try to access their own Service. Valid values are \"promiscuous-bridge\", \"hairpin-veth\" and \"none\".")
	fs.BoolVar(&s.BabysitDaemons, "babysit-daemons", s.BabysitDaemons, "If true, the node has babysitter process monitoring docker and kubelet.")
	fs.MarkDeprecated("babysit-daemons", "Will be removed in a future version.")
	fs.Int32Var(&s.MaxPods, "max-pods", s.MaxPods, "Number of Pods that can run on this Kubelet.")
	fs.StringVar(&s.DockerExecHandlerName, "docker-exec-handler", s.DockerExecHandlerName, "Handler to use when executing a command in a container. Valid values are 'native' and 'nsenter'. Defaults to 'native'.")
	fs.StringVar(&s.NonMasqueradeCIDR, "non-masquerade-cidr", s.NonMasqueradeCIDR, "Traffic to IPs outside this range will use IP masquerade.")
	fs.StringVar(&s.PodCIDR, "pod-cidr", "", "The CIDR to use for pod IP addresses, only used in standalone mode.  In cluster mode, this is obtained from the master.")
	fs.StringVar(&s.ResolverConfig, "resolv-conf", s.ResolverConfig, "Resolver configuration file used as the basis for the container DNS resolution configuration.")
	fs.BoolVar(&s.CPUCFSQuota, "cpu-cfs-quota", s.CPUCFSQuota, "Enable CPU CFS quota enforcement for containers that specify CPU limits")
	// Flags intended for testing, not recommended used in production environments.
	fs.BoolVar(&s.ReallyCrashForTesting, "really-crash-for-testing", s.ReallyCrashForTesting, "If true, when panics occur crash. Intended for testing.")
	fs.Float64Var(&s.ChaosChance, "chaos-chance", s.ChaosChance, "If > 0.0, introduce random client errors and latency. Intended for testing. [default=0.0]")
	fs.BoolVar(&s.Containerized, "containerized", s.Containerized, "Experimental support for running kubelet in a container.  Intended for testing. [default=false]")
	fs.Uint64Var(&s.MaxOpenFiles, "max-open-files", s.MaxOpenFiles, "Number of files that can be opened by Kubelet process. [default=1000000]")
	fs.BoolVar(&s.ReconcileCIDR, "reconcile-cidr", s.ReconcileCIDR, "Reconcile node CIDR with the CIDR specified by the API server. No-op if register-node or configure-cbr0 is false. [default=true]")
	fs.Var(&s.SystemReserved, "system-reserved", "A set of ResourceName=ResourceQuantity (e.g. cpu=200m,memory=150G) pairs that describe resources reserved for non-kubernetes components. Currently only cpu and memory are supported. See http://releases.k8s.io/HEAD/docs/user-guide/compute-resources.md for more detail. [default=none]")
	fs.Var(&s.KubeReserved, "kube-reserved", "A set of ResourceName=ResourceQuantity (e.g. cpu=200m,memory=150G) pairs that describe resources reserved for kubernetes system components. Currently only cpu and memory are supported. See http://releases.k8s.io/HEAD/docs/user-guide/compute-resources.md for more detail. [default=none]")
	fs.BoolVar(&s.RegisterSchedulable, "register-schedulable", s.RegisterSchedulable, "Register the node as schedulable. No-op if register-node is false. [default=true]")
	fs.StringVar(&s.ContentType, "kube-api-content-type", s.ContentType, "ContentType of requests sent to apiserver. Passing application/vnd.kubernetes.protobuf is an experimental feature now.")
	fs.Float32Var(&s.KubeAPIQPS, "kube-api-qps", s.KubeAPIQPS, "QPS to use while talking with kubernetes apiserver")
	fs.Int32Var(&s.KubeAPIBurst, "kube-api-burst", s.KubeAPIBurst, "Burst to use while talking with kubernetes apiserver")
	fs.BoolVar(&s.SerializeImagePulls, "serialize-image-pulls", s.SerializeImagePulls, "Pull images one at a time. We recommend *not* changing the default value on nodes that run docker daemon with version < 1.9 or an Aufs storage backend. Issue #10959 has more details. [default=true]")
	fs.BoolVar(&s.ExperimentalFlannelOverlay, "experimental-flannel-overlay", s.ExperimentalFlannelOverlay, "Experimental support for starting the kubelet with the default overlay network (flannel). Assumes flanneld is already running in client mode. [default=false]")
	fs.DurationVar(&s.OutOfDiskTransitionFrequency.Duration, "outofdisk-transition-frequency", s.OutOfDiskTransitionFrequency.Duration, "Duration for which the kubelet has to wait before transitioning out of out-of-disk node condition status. Default: 5m0s")
	fs.StringVar(&s.NodeIP, "node-ip", s.NodeIP, "IP address of the node. If set, kubelet will use this IP address for the node")
	fs.BoolVar(&s.EnableCustomMetrics, "enable-custom-metrics", s.EnableCustomMetrics, "Support for gathering custom metrics.")
	fs.StringVar(&s.RuntimeCgroups, "runtime-cgroups", s.RuntimeCgroups, "Optional absolute name of cgroups to create and run the runtime in.")
	fs.StringVar(&s.EvictionHard, "eviction-hard", s.EvictionHard, "A set of eviction thresholds (e.g. memory.available<1Gi) that if met would trigger a pod eviction.")
	fs.StringVar(&s.EvictionSoft, "eviction-soft", s.EvictionSoft, "A set of eviction thresholds (e.g. memory.available<1.5Gi) that if met over a corresponding grace period would trigger a pod eviction.")
	fs.StringVar(&s.EvictionSoftGracePeriod, "eviction-soft-grace-period", s.EvictionSoftGracePeriod, "A set of eviction grace periods (e.g. memory.available=1m30s) that correspond to how long a soft eviction threshold must hold before triggering a pod eviction.")
}

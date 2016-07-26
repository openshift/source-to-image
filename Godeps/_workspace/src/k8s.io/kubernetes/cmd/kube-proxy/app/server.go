/*
Copyright 2014 The Kubernetes Authors All rights reserved.

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

// Package app does all of the work necessary to configure and run a
// Kubernetes app process.
package app

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"strconv"
	"time"

	"k8s.io/kubernetes/cmd/kube-proxy/app/options"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/record"
	kubeclient "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/client/unversioned/clientcmd"
	clientcmdapi "k8s.io/kubernetes/pkg/client/unversioned/clientcmd/api"
	"k8s.io/kubernetes/pkg/proxy"
	proxyconfig "k8s.io/kubernetes/pkg/proxy/config"
	"k8s.io/kubernetes/pkg/proxy/iptables"
	"k8s.io/kubernetes/pkg/proxy/userspace"
	"k8s.io/kubernetes/pkg/types"
	"k8s.io/kubernetes/pkg/util"
	"k8s.io/kubernetes/pkg/util/configz"
	utildbus "k8s.io/kubernetes/pkg/util/dbus"
	"k8s.io/kubernetes/pkg/util/exec"
	utiliptables "k8s.io/kubernetes/pkg/util/iptables"
	utilnet "k8s.io/kubernetes/pkg/util/net"
	nodeutil "k8s.io/kubernetes/pkg/util/node"
	"k8s.io/kubernetes/pkg/util/oom"
	"k8s.io/kubernetes/pkg/util/wait"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type ProxyServer struct {
	Client       *kubeclient.Client
	Config       *options.ProxyServerConfig
	IptInterface utiliptables.Interface
	Proxier      proxy.ProxyProvider
	Broadcaster  record.EventBroadcaster
	Recorder     record.EventRecorder
	Conntracker  Conntracker // if nil, ignored
	ProxyMode    string
}

const (
	proxyModeUserspace              = "userspace"
	proxyModeIptables               = "iptables"
	experimentalProxyModeAnnotation = options.ExperimentalProxyModeAnnotation
	betaProxyModeAnnotation         = "net.beta.kubernetes.io/proxy-mode"
)

func checkKnownProxyMode(proxyMode string) bool {
	switch proxyMode {
	case "", proxyModeUserspace, proxyModeIptables:
		return true
	}
	return false
}

func NewProxyServer(
	client *kubeclient.Client,
	config *options.ProxyServerConfig,
	iptInterface utiliptables.Interface,
	proxier proxy.ProxyProvider,
	broadcaster record.EventBroadcaster,
	recorder record.EventRecorder,
	conntracker Conntracker,
	proxyMode string,
) (*ProxyServer, error) {
	return &ProxyServer{
		Client:       client,
		Config:       config,
		IptInterface: iptInterface,
		Proxier:      proxier,
		Broadcaster:  broadcaster,
		Recorder:     recorder,
		Conntracker:  conntracker,
		ProxyMode:    proxyMode,
	}, nil
}

// NewProxyCommand creates a *cobra.Command object with default parameters
func NewProxyCommand() *cobra.Command {
	s := options.NewProxyConfig()
	s.AddFlags(pflag.CommandLine)
	cmd := &cobra.Command{
		Use: "kube-proxy",
		Long: `The Kubernetes network proxy runs on each node. This
reflects services as defined in the Kubernetes API on each node and can do simple
TCP,UDP stream forwarding or round robin TCP,UDP forwarding across a set of backends.
Service cluster ips and ports are currently found through Docker-links-compatible
environment variables specifying ports opened by the service proxy. There is an optional
addon that provides cluster DNS for these cluster IPs. The user must create a service
with the apiserver API to configure the proxy.`,
		Run: func(cmd *cobra.Command, args []string) {
		},
	}

	return cmd
}

// NewProxyServerDefault creates a new ProxyServer object with default parameters.
func NewProxyServerDefault(config *options.ProxyServerConfig) (*ProxyServer, error) {
	if c, err := configz.New("componentconfig"); err == nil {
		c.Set(config.KubeProxyConfiguration)
	} else {
		glog.Errorf("unable to register configz: %s", err)
	}
	protocol := utiliptables.ProtocolIpv4
	if net.ParseIP(config.BindAddress).To4() == nil {
		protocol = utiliptables.ProtocolIpv6
	}

	// Create a iptables utils.
	execer := exec.New()
	dbus := utildbus.New()
	iptInterface := utiliptables.New(execer, dbus, protocol)

	// We omit creation of pretty much everything if we run in cleanup mode
	if config.CleanupAndExit {
		return &ProxyServer{
			Config:       config,
			IptInterface: iptInterface,
		}, nil
	}

	// TODO(vmarmol): Use container config for this.
	var oomAdjuster *oom.OOMAdjuster
	if config.OOMScoreAdj != nil {
		oomAdjuster = oom.NewOOMAdjuster()
		if err := oomAdjuster.ApplyOOMScoreAdj(0, int(*config.OOMScoreAdj)); err != nil {
			glog.V(2).Info(err)
		}
	}

	if config.ResourceContainer != "" {
		// Run in its own container.
		if err := util.RunInResourceContainer(config.ResourceContainer); err != nil {
			glog.Warningf("Failed to start in resource-only container %q: %v", config.ResourceContainer, err)
		} else {
			glog.V(2).Infof("Running in resource-only container %q", config.ResourceContainer)
		}
	}

	// Create a Kube Client
	// define api config source
	if config.Kubeconfig == "" && config.Master == "" {
		glog.Warningf("Neither --kubeconfig nor --master was specified.  Using default API client.  This might not work.")
	}
	// This creates a client, first loading any specified kubeconfig
	// file, and then overriding the Master flag, if non-empty.
	kubeconfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: config.Kubeconfig},
		&clientcmd.ConfigOverrides{ClusterInfo: clientcmdapi.Cluster{Server: config.Master}}).ClientConfig()
	if err != nil {
		return nil, err
	}

	kubeconfig.ContentType = config.ContentType
	// Override kubeconfig qps/burst settings from flags
	kubeconfig.QPS = config.KubeAPIQPS
	kubeconfig.Burst = int(config.KubeAPIBurst)

	client, err := kubeclient.New(kubeconfig)
	if err != nil {
		glog.Fatalf("Invalid API configuration: %v", err)
	}

	// Create event recorder
	hostname := nodeutil.GetHostname(config.HostnameOverride)
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(api.EventSource{Component: "kube-proxy", Host: hostname})

	var proxier proxy.ProxyProvider
	var endpointsHandler proxyconfig.EndpointsConfigHandler

	proxyMode := getProxyMode(string(config.Mode), client.Nodes(), hostname, iptInterface, iptables.LinuxKernelCompatTester{})
	if proxyMode == proxyModeIptables {
		glog.V(0).Info("Using iptables Proxier.")
		if config.IPTablesMasqueradeBit == nil {
			// IPTablesMasqueradeBit must be specified or defaulted.
			return nil, fmt.Errorf("Unable to read IPTablesMasqueradeBit from config")
		}

		proxierIptables, err := iptables.NewProxier(iptInterface, execer, config.IPTablesSyncPeriod.Duration, config.MasqueradeAll, int(*config.IPTablesMasqueradeBit), config.ClusterCIDR)
		if err != nil {
			glog.Fatalf("Unable to create proxier: %v", err)
		}
		proxier = proxierIptables
		endpointsHandler = proxierIptables
		// No turning back. Remove artifacts that might still exist from the userspace Proxier.
		glog.V(0).Info("Tearing down userspace rules.")
		userspace.CleanupLeftovers(iptInterface)
	} else {
		glog.V(0).Info("Using userspace Proxier.")
		// This is a proxy.LoadBalancer which NewProxier needs but has methods we don't need for
		// our config.EndpointsConfigHandler.
		loadBalancer := userspace.NewLoadBalancerRR()
		// set EndpointsConfigHandler to our loadBalancer
		endpointsHandler = loadBalancer

		proxierUserspace, err := userspace.NewProxier(
			loadBalancer,
			net.ParseIP(config.BindAddress),
			iptInterface,
			*utilnet.ParsePortRangeOrDie(config.PortRange),
			config.IPTablesSyncPeriod.Duration,
			config.UDPIdleTimeout.Duration,
		)
		if err != nil {
			glog.Fatalf("Unable to create proxier: %v", err)
		}
		proxier = proxierUserspace
		// Remove artifacts from the pure-iptables Proxier.
		glog.V(0).Info("Tearing down pure-iptables proxy rules.")
		iptables.CleanupLeftovers(iptInterface)
	}
	iptInterface.AddReloadFunc(proxier.Sync)

	// Create configs (i.e. Watches for Services and Endpoints)
	// Note: RegisterHandler() calls need to happen before creation of Sources because sources
	// only notify on changes, and the initial update (on process start) may be lost if no handlers
	// are registered yet.
	serviceConfig := proxyconfig.NewServiceConfig()
	serviceConfig.RegisterHandler(proxier)

	endpointsConfig := proxyconfig.NewEndpointsConfig()
	endpointsConfig.RegisterHandler(endpointsHandler)

	proxyconfig.NewSourceAPI(
		client,
		config.ConfigSyncPeriod,
		serviceConfig.Channel("api"),
		endpointsConfig.Channel("api"),
	)

	config.NodeRef = &api.ObjectReference{
		Kind:      "Node",
		Name:      hostname,
		UID:       types.UID(hostname),
		Namespace: "",
	}

	conntracker := realConntracker{}

	return NewProxyServer(client, config, iptInterface, proxier, eventBroadcaster, recorder, conntracker, proxyMode)
}

// Run runs the specified ProxyServer.  This should never exit (unless CleanupAndExit is set).
func (s *ProxyServer) Run() error {
	// remove iptables rules and exit
	if s.Config.CleanupAndExit {
		encounteredError := userspace.CleanupLeftovers(s.IptInterface)
		encounteredError = iptables.CleanupLeftovers(s.IptInterface) || encounteredError
		if encounteredError {
			return errors.New("Encountered an error while tearing down rules.")
		}
		return nil
	}

	s.Broadcaster.StartRecordingToSink(s.Client.Events(""))

	// Start up a webserver if requested
	if s.Config.HealthzPort > 0 {
		http.HandleFunc("/proxyMode", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "%s", s.ProxyMode)
		})
		configz.InstallHandler(http.DefaultServeMux)
		go wait.Until(func() {
			err := http.ListenAndServe(s.Config.HealthzBindAddress+":"+strconv.Itoa(int(s.Config.HealthzPort)), nil)
			if err != nil {
				glog.Errorf("Starting health server failed: %v", err)
			}
		}, 5*time.Second, wait.NeverStop)
	}

	// Tune conntrack, if requested
	if s.Conntracker != nil {
		if s.Config.ConntrackMax > 0 {
			if err := s.Conntracker.SetMax(int(s.Config.ConntrackMax)); err != nil {
				return err
			}
		}
		if s.Config.ConntrackTCPEstablishedTimeout.Duration > 0 {
			if err := s.Conntracker.SetTCPEstablishedTimeout(int(s.Config.ConntrackTCPEstablishedTimeout.Duration / time.Second)); err != nil {
				return err
			}
		}
	}

	// Birth Cry after the birth is successful
	s.birthCry()

	// Just loop forever for now...
	s.Proxier.SyncLoop()
	return nil
}

type nodeGetter interface {
	Get(hostname string) (*api.Node, error)
}

func getProxyMode(proxyMode string, client nodeGetter, hostname string, iptver iptables.IptablesVersioner, kcompat iptables.KernelCompatTester) string {
	if proxyMode == proxyModeUserspace {
		return proxyModeUserspace
	} else if proxyMode == proxyModeIptables {
		return tryIptablesProxy(iptver, kcompat)
	} else if proxyMode != "" {
		glog.V(1).Infof("Flag proxy-mode=%q unknown, assuming iptables proxy", proxyMode)
		return tryIptablesProxy(iptver, kcompat)
	}
	// proxyMode == "" - choose the best option.
	if client == nil {
		glog.Errorf("nodeGetter is nil: assuming iptables proxy")
		return tryIptablesProxy(iptver, kcompat)
	}
	node, err := client.Get(hostname)
	if err != nil {
		glog.Errorf("Can't get Node %q, assuming iptables proxy, err: %v", hostname, err)
		return tryIptablesProxy(iptver, kcompat)
	}
	if node == nil {
		glog.Errorf("Got nil Node %q, assuming iptables proxy", hostname)
		return tryIptablesProxy(iptver, kcompat)
	}
	proxyMode, found := node.Annotations[betaProxyModeAnnotation]
	if found {
		glog.V(1).Infof("Found beta annotation %q = %q", betaProxyModeAnnotation, proxyMode)
	} else {
		// We already published some information about this annotation with the "experimental" name, so we will respect it.
		proxyMode, found = node.Annotations[experimentalProxyModeAnnotation]
		if found {
			glog.V(1).Infof("Found experimental annotation %q = %q", experimentalProxyModeAnnotation, proxyMode)
		}
	}
	if proxyMode == proxyModeUserspace {
		glog.V(1).Infof("Annotation demands userspace proxy")
		return proxyModeUserspace
	}
	return tryIptablesProxy(iptver, kcompat)
}

func tryIptablesProxy(iptver iptables.IptablesVersioner, kcompat iptables.KernelCompatTester) string {
	var err error
	// guaranteed false on error, error only necessary for debugging
	useIptablesProxy, err := iptables.CanUseIptablesProxier(iptver, kcompat)
	if err != nil {
		glog.Errorf("Can't determine whether to use iptables proxy, using userspace proxier: %v", err)
		return proxyModeUserspace
	}
	if useIptablesProxy {
		return proxyModeIptables
	}
	// Fallback.
	glog.V(1).Infof("Can't use iptables proxy, using userspace proxier: %v", err)
	return proxyModeUserspace
}

func (s *ProxyServer) birthCry() {
	s.Recorder.Eventf(s.Config.NodeRef, api.EventTypeNormal, "Starting", "Starting kube-proxy.")
}

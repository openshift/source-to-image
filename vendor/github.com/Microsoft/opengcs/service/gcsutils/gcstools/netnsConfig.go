package main

// This utility moves a network interface into a network namespace and
// configures it. The configuration is passed in as a JSON object
// (marshalled prot.NetworkAdapter).  It is necessary to implement
// this as a separate utility as in Go one does not have tight control
// over which OS thread a given Go thread/routing executes but as can
// only enter a namespace with a specific OS thread.
//
// Note, this logs to stdout so that the caller (gcs) can log the
// output itself.

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/Microsoft/opengcs/service/gcs/prot"
	"github.com/Microsoft/opengcs/service/gcsutils/gcstools/commoncli"
	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
)

func netnsConfigMain() {
	if err := netnsConfig(); err != nil {
		log.Errorf("netnsConfig returned: %s", err)
		fmt.Fprintf(os.Stderr, err.Error())
		os.Exit(-1)
	}
	log.Info("netnsConfig succeeded")
	os.Exit(0)
}

func netnsConfig() error {
	ifStr := flag.String("if", "", "Interface/Adapter to move/configure")
	nspid := flag.Int("nspid", -1, "Process ID (to locate netns")
	cfgStr := flag.String("cfg", "", "Adapter configuration (json)")
	logArgs := commoncli.SetFlagsForLogging()

	flag.Parse()
	if err := commoncli.SetupLogging(logArgs...); err != nil {
		return err
	}
	if *ifStr == "" || *nspid == -1 || *cfgStr == "" {
		return fmt.Errorf("All three arguments must be specified")
	}

	var a prot.NetworkAdapter
	if err := json.Unmarshal([]byte(*cfgStr), &a); err != nil {
		return err
	}

	if a.NatEnabled {
		log.Infof("Configure %s in %d with: %s/%d gw=%s", *ifStr, *nspid, a.AllocatedIPAddress, a.HostIPPrefixLength, a.HostIPAddress)
	} else {
		log.Infof("Configure %s in %d with DHCP", *ifStr, *nspid)
	}

	// Lock the OS Thread so we don't accidentally switch namespaces
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Stash current network namespace away and make sure we enter it as we leave
	log.Infof("Obtaining current namespace")
	origNS, err := netns.Get()
	if err != nil {
		return fmt.Errorf("netns.Get() failed: %v", err)
	}
	defer origNS.Close()
	log.Infof("Original namespace %v", origNS)

	// Get a reference to the new network namespace
	ns, err := netns.GetFromPid(*nspid)
	if err != nil {
		return fmt.Errorf("netns.GetFromPid(%d) failed: %v", *nspid, err)
	}
	defer ns.Close()
	log.Infof("New network namespace from PID %d is %v", *nspid, ns)

	// Get a reference to the interface and make sure it's down
	link, err := netlink.LinkByName(*ifStr)
	if err != nil {
		return fmt.Errorf("netlink.LinkByName(%s) failed: %v", *ifStr, err)
	}
	if err := netlink.LinkSetDown(link); err != nil {
		return fmt.Errorf("netlink.LinkSetDown(%#v) failed: %v", link, err)
	}

	// Move the interface to the new network namespace
	if err := netlink.LinkSetNsPid(link, *nspid); err != nil {
		return fmt.Errorf("netlink.SetNsPid(%#v, %d) failed: %v", link, *nspid, err)
	}

	log.Infof("Switching from %v to %v", origNS, ns)

	// Enter the new network namespace
	if err := netns.Set(ns); err != nil {
		return fmt.Errorf("netns.Set() failed: %v", err)
	}

	// Re-Get a reference to the interface (it may be a different ID in the new namespace)
	log.Infof("Getting reference to interface")
	link, err = netlink.LinkByName(*ifStr)
	if err != nil {
		return fmt.Errorf("netlink.LinkByName(%s) failed: %v", *ifStr, err)
	}

	// User requested non-default MTU size
	if a.EncapOverhead != 0 {
		log.Info("EncapOverhead non-zero, will set MTU")
		mtu := link.Attrs().MTU - int(a.EncapOverhead)
		log.Infof("mtu %d", mtu)
		if err = netlink.LinkSetMTU(link, mtu); err != nil {
			return fmt.Errorf("netlink.LinkSetMTU(%#v, %d) failed: %v", link, mtu, err)
		}
	}

	// Configure the interface
	if a.NatEnabled {
		log.Info("Nat enabled - configuring interface")
		metric := 1
		if a.EnableLowMetric {
			metric = 500
		}

		// Bring the interface up
		if err := netlink.LinkSetUp(link); err != nil {
			return fmt.Errorf("netlink.LinkSetUp(%#v) failed: %v", link, err)
		}
		// Set IP address
		addr := &net.IPNet{
			IP: net.ParseIP(a.AllocatedIPAddress),
			// TODO(rn): This assumes/hardcodes IPv4
			Mask: net.CIDRMask(int(a.HostIPPrefixLength), 32)}
		ipAddr := &netlink.Addr{IPNet: addr, Label: ""}
		if err := netlink.AddrAdd(link, ipAddr); err != nil {
			return fmt.Errorf("netlink.AddrAdd(%#v, %#v) failed: %v", link, ipAddr, err)
		}
		// Set gateway
		if a.HostIPAddress != "" {
			gw := net.ParseIP(a.HostIPAddress)

			if !addr.Contains(gw) {
				// In the case that a gw is not part of the subnet we are setting gw for,
				// a new addr containing this gw address need to be added into the link to avoid getting
				// unreachable error when adding this out-of-subnet gw route
				log.Infof("gw is outside of the subnet: Configure %s in %d with: %s/%d gw=%s\n",
					*ifStr, *nspid, a.AllocatedIPAddress, a.HostIPPrefixLength, a.HostIPAddress)
				addr2 := &net.IPNet{
					IP:   net.ParseIP(a.HostIPAddress),
					Mask: net.CIDRMask(32, 32)} // This assumes/hardcodes IPv4
				ipAddr2 := &netlink.Addr{IPNet: addr2, Label: ""}
				if err := netlink.AddrAdd(link, ipAddr2); err != nil {
					return fmt.Errorf("netlink.AddrAdd(%#v, %#v) failed: %v", link, ipAddr2, err)
				}
			}
			route := netlink.Route{
				Scope:     netlink.SCOPE_UNIVERSE,
				LinkIndex: link.Attrs().Index,
				Gw:        gw,
				Priority:  metric, // This is what ip route add does
			}
			if err := netlink.RouteAdd(&route); err != nil {
				return fmt.Errorf("netlink.RouteAdd(%#v) failed: %v", route, err)
			}
		}
	} else {
		log.Infof("Execing udhcpc with timeout...")
		cmd := exec.Command("udhcpc", "-q", "-i", *ifStr, "-s", "/sbin/udhcpc_config.script")

		done := make(chan error)
		go func() {
			done <- cmd.Wait()
		}()
		defer close(done)

		select {
		case <-time.After(30 * time.Second):
			var cos string
			co, err := cmd.CombinedOutput() // In case it has written something
			if err != nil {
				cos = string(co)
			}
			cmd.Process.Kill()
			log.Infof("udhcpc timed out [%s]", cos)
			return fmt.Errorf("udhcpc timed out. Failed to get DHCP address: %s", cos)
		case err := <-done:
			var cos string
			co, err := cmd.CombinedOutput() // Something should be on stderr
			if err != nil {
				cos = string(co)
			}
			if err != nil {
				log.Infof("udhcpc failed %s [%s]", err, cos)
				return fmt.Errorf("process failed: %s (%s)", err, cos)
			}
		}
		var cos string
		co, err := cmd.CombinedOutput()
		if err != nil {
			cos = string(co)
		}
		log.Debugf("udhcpc succeeded: %s", cos)
	}

	// Add some debug logging
	curNS, _ := netns.Get()
	// Refresh link attributes/state
	link, _ = netlink.LinkByIndex(link.Attrs().Index)
	attr := link.Attrs()
	addrs, _ := netlink.AddrList(link, 0)
	log.Infof("%v: %s[idx=%d,type=%s] is %v", curNS, attr.Name, attr.Index, link.Type(), attr.OperState)
	for _, addr := range addrs {
		log.Infof("  %v", addr)
	}

	return nil
}

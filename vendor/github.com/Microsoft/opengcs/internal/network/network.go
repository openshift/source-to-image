// +build linux

package network

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Microsoft/opengcs/internal/log"
	"github.com/Microsoft/opengcs/internal/oc"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
)

// maxDNSSearches is limited to 6 in `man 5 resolv.conf`
const maxDNSSearches = 6

// GenerateEtcHostsContent generates a /etc/hosts file based on `hostname`.
func GenerateEtcHostsContent(ctx context.Context, hostname string) string {
	_, span := trace.StartSpan(ctx, "network::GenerateEtcHostsContent")
	defer span.End()
	span.AddAttributes(
		trace.StringAttribute("hostname", hostname))

	nameParts := strings.Split(hostname, ".")
	buf := bytes.Buffer{}
	buf.WriteString("127.0.0.1 localhost\n")
	if len(nameParts) > 1 {
		buf.WriteString(fmt.Sprintf("127.0.0.1 %s %s\n", hostname, nameParts[0]))
	} else {
		buf.WriteString(fmt.Sprintf("127.0.0.1 %s\n", hostname))
	}
	buf.WriteString("\n")
	buf.WriteString("# The following lines are desirable for IPv6 capable hosts\n")
	buf.WriteString("::1     ip6-localhost ip6-loopback\n")
	buf.WriteString("fe00::0 ip6-localnet\n")
	buf.WriteString("ff00::0 ip6-mcastprefix\n")
	buf.WriteString("ff02::1 ip6-allnodes\n")
	buf.WriteString("ff02::2 ip6-allrouters\n")
	return buf.String()
}

// GenerateResolvConfContent generates the resolv.conf file content based on
// `searches`, `servers`, and `options`.
func GenerateResolvConfContent(ctx context.Context, searches, servers, options []string) (_ string, err error) {
	_, span := trace.StartSpan(ctx, "network::GenerateResolvConfContent")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("searches", strings.Join(searches, ", ")),
		trace.StringAttribute("servers", strings.Join(servers, ", ")),
		trace.StringAttribute("options", strings.Join(options, ", ")))

	if len(searches) > maxDNSSearches {
		return "", errors.Errorf("searches has more than %d domains", maxDNSSearches)
	}

	content := ""
	if len(searches) > 0 {
		content += fmt.Sprintf("search %s\n", strings.Join(searches, " "))
	}
	if len(servers) > 0 {
		content += fmt.Sprintf("nameserver %s\n", strings.Join(servers, "\nnameserver "))
	}
	if len(options) > 0 {
		content += fmt.Sprintf("options %s\n", strings.Join(options, " "))
	}
	return content, nil
}

// MergeValues merges `first` and `second` maintaining order `first, second`.
func MergeValues(first, second []string) []string {
	if len(first) == 0 {
		return second
	}
	if len(second) == 0 {
		return first
	}
	values := make([]string, len(first), len(first)+len(second))
	copy(values, first)
	for _, v := range second {
		found := false
		for i := 0; i < len(values); i++ {
			if v == values[i] {
				found = true
				break
			}
		}
		if !found {
			values = append(values, v)
		}
	}
	return values
}

// InstanceIDToName converts from the given instance ID (a GUID generated on the
// Windows host) to its corresponding interface name (e.g. "eth0").
func InstanceIDToName(ctx context.Context, id string, wait bool) (_ string, err error) {
	ctx, span := trace.StartSpan(ctx, "network::InstanceIDToName")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	id = strings.ToLower(id)
	span.AddAttributes(
		trace.StringAttribute("adapterInstanceID", id),
		trace.BoolAttribute("wait", wait))

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
	log.G(ctx).WithField("ifname", ifname).Debug("resolved ifname")
	return ifname, nil
}

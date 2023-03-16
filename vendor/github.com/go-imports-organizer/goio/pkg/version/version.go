package version

import (
	"runtime/debug"
)

var Version string

// Get returns the applications Version based on its build information
func Get() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		return info.Main.Version
	}
	return "unknown"
}

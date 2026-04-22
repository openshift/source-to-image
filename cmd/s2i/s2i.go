package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"time"

	"k8s.io/klog/v2"

	"github.com/openshift/source-to-image/pkg/cmd/cli"
)

func init() {
	klog.InitFlags(flag.CommandLine)

	// Opt into the new klog behavior so that -stderrthreshold is honored even
	// when -logtostderr=true (the default).
	// Ref: kubernetes/klog#212, kubernetes/klog#432
	flag.CommandLine.Set("legacy_stderr_threshold_behavior", "false") //nolint:errcheck
	flag.CommandLine.Set("stderrthreshold", "INFO")                  //nolint:errcheck
}

func main() {
	rand.Seed(time.Now().UTC().UnixNano())
	if len(os.Getenv("GOMAXPROCS")) == 0 {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	command := cli.CommandFor()
	if err := command.Execute(); err != nil {
		fmt.Println(fmt.Sprintf("S2I encountered the following error: %v", err))
		os.Exit(1)
	}
}

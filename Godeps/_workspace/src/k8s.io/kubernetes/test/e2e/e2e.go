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

package e2e

import (
	"fmt"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/golang/glog"
	"github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	"github.com/onsi/ginkgo/reporters"
	"github.com/onsi/gomega"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"k8s.io/kubernetes/pkg/api"
	gcecloud "k8s.io/kubernetes/pkg/cloudprovider/providers/gce"
	"k8s.io/kubernetes/pkg/util"
	"k8s.io/kubernetes/pkg/util/runtime"
	"k8s.io/kubernetes/test/e2e/framework"
)

const (
	// podStartupTimeout is the time to allow all pods in the cluster to become
	// running and ready before any e2e tests run. It includes pulling all of
	// the pods (as of 5/18/15 this is 8 pods).
	podStartupTimeout = 10 * time.Minute
)

var (
	cloudConfig = &framework.TestContext.CloudConfig
)

// setupProviderConfig validates and sets up cloudConfig based on framework.TestContext.Provider.
func setupProviderConfig() error {
	switch framework.TestContext.Provider {
	case "":
		glog.Info("The --provider flag is not set.  Treating as a conformance test.  Some tests may not be run.")

	case "gce", "gke":
		var err error
		framework.Logf("Fetching cloud provider for %q\r\n", framework.TestContext.Provider)
		var tokenSource oauth2.TokenSource
		tokenSource = nil
		if cloudConfig.ServiceAccount != "" {
			// Use specified service account for auth
			framework.Logf("Using service account %q as token source.", cloudConfig.ServiceAccount)
			tokenSource = google.ComputeTokenSource(cloudConfig.ServiceAccount)
		}
		zone := framework.TestContext.CloudConfig.Zone
		region, err := gcecloud.GetGCERegion(zone)
		if err != nil {
			return fmt.Errorf("error parsing GCE/GKE region from zone %q: %v", zone, err)
		}
		managedZones := []string{zone} // Only single-zone for now
		cloudConfig.Provider, err = gcecloud.CreateGCECloud(framework.TestContext.CloudConfig.ProjectID, region, zone, managedZones, "" /* networkUrl */, nil /* nodeTags */, tokenSource, false /* useMetadataServer */)
		if err != nil {
			return fmt.Errorf("Error building GCE/GKE provider: %v", err)
		}

	case "aws":
		if cloudConfig.Zone == "" {
			return fmt.Errorf("gce-zone must be specified for AWS")
		}

	}

	return nil
}

// There are certain operations we only want to run once per overall test invocation
// (such as deleting old namespaces, or verifying that all system pods are running.
// Because of the way Ginkgo runs tests in parallel, we must use SynchronizedBeforeSuite
// to ensure that these operations only run on the first parallel Ginkgo node.
//
// This function takes two parameters: one function which runs on only the first Ginkgo node,
// returning an opaque byte array, and then a second function which runs on all Ginkgo nodes,
// accepting the byte array.
var _ = ginkgo.SynchronizedBeforeSuite(func() []byte {
	// Run only on Ginkgo node 1

	// Delete any namespaces except default and kube-system. This ensures no
	// lingering resources are left over from a previous test run.
	if framework.TestContext.CleanStart {
		c, err := framework.LoadClient()
		if err != nil {
			glog.Fatal("Error loading client: ", err)
		}

		deleted, err := framework.DeleteNamespaces(c, nil /* deleteFilter */, []string{api.NamespaceSystem, api.NamespaceDefault})
		if err != nil {
			framework.Failf("Error deleting orphaned namespaces: %v", err)
		}
		glog.Infof("Waiting for deletion of the following namespaces: %v", deleted)
		if err := framework.WaitForNamespacesDeleted(c, deleted, framework.NamespaceCleanupTimeout); err != nil {
			framework.Failf("Failed to delete orphaned namespaces %v: %v", deleted, err)
		}
	}

	// Ensure all pods are running and ready before starting tests (otherwise,
	// cluster infrastructure pods that are being pulled or started can block
	// test pods from running, and tests that ensure all pods are running and
	// ready will fail).
	if err := framework.WaitForPodsRunningReady(api.NamespaceSystem, int32(framework.TestContext.MinStartupPods), podStartupTimeout); err != nil {
		if c, errClient := framework.LoadClient(); errClient != nil {
			framework.Logf("Unable to dump cluster information because: %v", errClient)
		} else {
			framework.DumpAllNamespaceInfo(c, api.NamespaceSystem)
		}
		framework.LogFailedContainers(api.NamespaceSystem)
		framework.RunKubernetesServiceTestContainer(framework.TestContext.RepoRoot, api.NamespaceDefault)
		framework.Failf("Error waiting for all pods to be running and ready: %v", err)
	}

	return nil

}, func(data []byte) {
	// Run on all Ginkgo nodes

})

type CleanupActionHandle *int

var cleanupActionsLock sync.Mutex
var cleanupActions = map[CleanupActionHandle]func(){}

// AddCleanupAction installs a function that will be called in the event of the
// whole test being terminated.  This allows arbitrary pieces of the overall
// test to hook into SynchronizedAfterSuite().
func AddCleanupAction(fn func()) CleanupActionHandle {
	p := CleanupActionHandle(new(int))
	cleanupActionsLock.Lock()
	defer cleanupActionsLock.Unlock()
	cleanupActions[p] = fn
	return p
}

// RemoveCleanupAction removes a function that was installed by
// AddCleanupAction.
func RemoveCleanupAction(p CleanupActionHandle) {
	cleanupActionsLock.Lock()
	defer cleanupActionsLock.Unlock()
	delete(cleanupActions, p)
}

// RunCleanupActions runs all functions installed by AddCleanupAction.  It does
// not remove them (see RemoveCleanupAction) but it does run unlocked, so they
// may remove themselves.
func RunCleanupActions() {
	list := []func(){}
	func() {
		cleanupActionsLock.Lock()
		defer cleanupActionsLock.Unlock()
		for _, fn := range cleanupActions {
			list = append(list, fn)
		}
	}()
	// Run unlocked.
	for _, fn := range list {
		fn()
	}
}

// Similar to SynchornizedBeforeSuite, we want to run some operations only once (such as collecting cluster logs).
// Here, the order of functions is reversed; first, the function which runs everywhere,
// and then the function that only runs on the first Ginkgo node.
var _ = ginkgo.SynchronizedAfterSuite(func() {
	// Run on all Ginkgo nodes
	RunCleanupActions()
}, func() {
	// Run only Ginkgo on node 1
	if framework.TestContext.ReportDir != "" {
		framework.CoreDump(framework.TestContext.ReportDir)
	}
})

// TestE2E checks configuration parameters (specified through flags) and then runs
// E2E tests using the Ginkgo runner.
// If a "report directory" is specified, one or more JUnit test reports will be
// generated in this directory, and cluster logs will also be saved.
// This function is called on each Ginkgo node in parallel mode.
func RunE2ETests(t *testing.T) {
	runtime.ReallyCrash = true
	util.InitLogs()
	defer util.FlushLogs()

	// We must call setupProviderConfig first since SynchronizedBeforeSuite needs
	// cloudConfig to be set up already.
	if err := setupProviderConfig(); err != nil {
		glog.Fatalf(err.Error())
	}

	gomega.RegisterFailHandler(ginkgo.Fail)
	// Disable skipped tests unless they are explicitly requested.
	if config.GinkgoConfig.FocusString == "" && config.GinkgoConfig.SkipString == "" {
		config.GinkgoConfig.SkipString = `\[Flaky\]|\[Feature:.+\]`
	}

	// Run tests through the Ginkgo runner with output to console + JUnit for Jenkins
	var r []ginkgo.Reporter
	if framework.TestContext.ReportDir != "" {
		// TODO: we should probably only be trying to create this directory once
		// rather than once-per-Ginkgo-node.
		if err := os.MkdirAll(framework.TestContext.ReportDir, 0755); err != nil {
			glog.Errorf("Failed creating report directory: %v", err)
		} else {
			r = append(r, reporters.NewJUnitReporter(path.Join(framework.TestContext.ReportDir, fmt.Sprintf("junit_%v%02d.xml", framework.TestContext.ReportPrefix, config.GinkgoConfig.ParallelNode))))
		}
	}
	glog.Infof("Starting e2e run %q on Ginkgo node %d", framework.RunId, config.GinkgoConfig.ParallelNode)
	ginkgo.RunSpecsWithDefaultAndCustomReporters(t, "Kubernetes e2e suite", r)
}

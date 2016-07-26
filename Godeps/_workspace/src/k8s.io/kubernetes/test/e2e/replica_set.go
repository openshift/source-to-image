/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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
	"time"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/apis/extensions"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/util"
	"k8s.io/kubernetes/pkg/util/wait"
	"k8s.io/kubernetes/test/e2e/framework"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = framework.KubeDescribe("ReplicaSet", func() {
	f := framework.NewDefaultFramework("replicaset")

	It("should serve a basic image on each replica with a public image [Conformance]", func() {
		ReplicaSetServeImageOrFail(f, "basic", "gcr.io/google_containers/serve_hostname:v1.4")
	})

	It("should serve a basic image on each replica with a private image", func() {
		// requires private images
		framework.SkipUnlessProviderIs("gce", "gke")

		ReplicaSetServeImageOrFail(f, "private", "b.gcr.io/k8s_authenticated_test/serve_hostname:v1.4")
	})
})

// A basic test to check the deployment of an image using a ReplicaSet. The
// image serves its hostname which is checked for each replica.
func ReplicaSetServeImageOrFail(f *framework.Framework, test string, image string) {
	name := "my-hostname-" + test + "-" + string(util.NewUUID())
	replicas := int32(2)

	// Create a ReplicaSet for a service that serves its hostname.
	// The source for the Docker containter kubernetes/serve_hostname is
	// in contrib/for-demos/serve_hostname
	By(fmt.Sprintf("Creating ReplicaSet %s", name))
	rs, err := f.Client.Extensions().ReplicaSets(f.Namespace.Name).Create(&extensions.ReplicaSet{
		ObjectMeta: api.ObjectMeta{
			Name: name,
		},
		Spec: extensions.ReplicaSetSpec{
			Replicas: replicas,
			Selector: &unversioned.LabelSelector{MatchLabels: map[string]string{
				"name": name,
			}},
			Template: api.PodTemplateSpec{
				ObjectMeta: api.ObjectMeta{
					Labels: map[string]string{"name": name},
				},
				Spec: api.PodSpec{
					Containers: []api.Container{
						{
							Name:  name,
							Image: image,
							Ports: []api.ContainerPort{{ContainerPort: 9376}},
						},
					},
				},
			},
		},
	})
	Expect(err).NotTo(HaveOccurred())
	// Cleanup the ReplicaSet when we are done.
	defer func() {
		// Resize the ReplicaSet to zero to get rid of pods.
		if err := framework.DeleteReplicaSet(f.Client, f.Namespace.Name, rs.Name); err != nil {
			framework.Logf("Failed to cleanup ReplicaSet %v: %v.", rs.Name, err)
		}
	}()

	// List the pods, making sure we observe all the replicas.
	label := labels.SelectorFromSet(labels.Set(map[string]string{"name": name}))

	pods, err := framework.PodsCreated(f.Client, f.Namespace.Name, name, replicas)
	Expect(err).NotTo(HaveOccurred())

	By("Ensuring each pod is running")

	// Wait for the pods to enter the running state. Waiting loops until the pods
	// are running so non-running pods cause a timeout for this test.
	for _, pod := range pods.Items {
		if pod.DeletionTimestamp != nil {
			continue
		}
		err = f.WaitForPodRunning(pod.Name)
		Expect(err).NotTo(HaveOccurred())
	}

	// Verify that something is listening.
	By("Trying to dial each unique pod")
	retryTimeout := 2 * time.Minute
	retryInterval := 5 * time.Second
	err = wait.Poll(retryInterval, retryTimeout, framework.PodProxyResponseChecker(f.Client, f.Namespace.Name, label, name, true, pods).CheckAllResponses)
	if err != nil {
		framework.Failf("Did not get expected responses within the timeout period of %.2f seconds.", retryTimeout.Seconds())
	}
}

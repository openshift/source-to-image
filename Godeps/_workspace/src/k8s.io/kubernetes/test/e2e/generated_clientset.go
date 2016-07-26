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

package e2e

import (
	"fmt"
	"strconv"
	"time"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/util"
	"k8s.io/kubernetes/pkg/util/intstr"
	"k8s.io/kubernetes/pkg/watch"
	"k8s.io/kubernetes/test/e2e/framework"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func testingPod(name, value string) v1.Pod {
	return v1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"name": "foo",
				"time": value,
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "nginx",
					Image: "gcr.io/google_containers/nginx:1.7.9",
					Ports: []v1.ContainerPort{{ContainerPort: 80}},
					LivenessProbe: &v1.Probe{
						Handler: v1.Handler{
							HTTPGet: &v1.HTTPGetAction{
								Path: "/index.html",
								Port: intstr.FromInt(8080),
							},
						},
						InitialDelaySeconds: 30,
					},
				},
			},
		},
	}
}

func observePodCreation(w watch.Interface) {
	select {
	case event, _ := <-w.ResultChan():
		if event.Type != watch.Added {
			framework.Failf("Failed to observe pod creation: %v", event)
		}
	case <-time.After(framework.PodStartTimeout):
		Fail("Timeout while waiting for pod creation")
	}
}

func observePodDeletion(w watch.Interface) (lastPod *api.Pod) {
	deleted := false
	timeout := false
	timer := time.After(60 * time.Second)
	for !deleted && !timeout {
		select {
		case event, _ := <-w.ResultChan():
			if event.Type == watch.Deleted {
				lastPod = event.Object.(*api.Pod)
				deleted = true
			}
		case <-timer:
			timeout = true
		}
	}
	if !deleted {
		Fail("Failed to observe pod deletion")
	}
	return
}

var _ = framework.KubeDescribe("Generated release_1_2 clientset", func() {
	f := framework.NewDefaultFramework("clientset")
	It("should create pods, delete pods, watch pods", func() {
		podClient := f.Clientset_1_2.Core().Pods(f.Namespace.Name)
		By("constructing the pod")
		name := "pod" + string(util.NewUUID())
		value := strconv.Itoa(time.Now().Nanosecond())
		podCopy := testingPod(name, value)
		pod := &podCopy
		By("setting up watch")
		selector := labels.SelectorFromSet(labels.Set(map[string]string{"time": value}))
		options := api.ListOptions{LabelSelector: selector}
		pods, err := podClient.List(options)
		if err != nil {
			framework.Failf("Failed to query for pods: %v", err)
		}
		Expect(len(pods.Items)).To(Equal(0))
		options = api.ListOptions{
			LabelSelector:   selector,
			ResourceVersion: pods.ListMeta.ResourceVersion,
		}
		w, err := podClient.Watch(options)
		if err != nil {
			framework.Failf("Failed to set up watch: %v", err)
		}

		By("creating the pod")
		pod, err = podClient.Create(pod)
		if err != nil {
			framework.Failf("Failed to create pod: %v", err)
		}
		// We call defer here in case there is a problem with
		// the test so we can ensure that we clean up after
		// ourselves
		defer podClient.Delete(pod.Name, api.NewDeleteOptions(0))

		By("verifying the pod is in kubernetes")
		options = api.ListOptions{
			LabelSelector:   selector,
			ResourceVersion: pod.ResourceVersion,
		}
		pods, err = podClient.List(options)
		if err != nil {
			framework.Failf("Failed to query for pods: %v", err)
		}
		Expect(len(pods.Items)).To(Equal(1))

		By("verifying pod creation was observed")
		observePodCreation(w)

		// We need to wait for the pod to be scheduled, otherwise the deletion
		// will be carried out immediately rather than gracefully.
		framework.ExpectNoError(f.WaitForPodRunning(pod.Name))

		By("deleting the pod gracefully")
		if err := podClient.Delete(pod.Name, api.NewDeleteOptions(30)); err != nil {
			framework.Failf("Failed to delete pod: %v", err)
		}

		By("verifying pod deletion was observed")
		lastPod := observePodDeletion(w)
		Expect(lastPod.DeletionTimestamp).ToNot(BeNil())
		Expect(lastPod.Spec.TerminationGracePeriodSeconds).ToNot(BeZero())

		options = api.ListOptions{LabelSelector: selector}
		pods, err = podClient.List(options)
		if err != nil {
			Fail(fmt.Sprintf("Failed to list pods to verify deletion: %v", err))
		}
		Expect(len(pods.Items)).To(Equal(0))
	})
})

var _ = framework.KubeDescribe("Generated release_1_3 clientset", func() {
	f := framework.NewDefaultFramework("clientset")
	It("should create pods, delete pods, watch pods", func() {
		podClient := f.Clientset_1_3.Core().Pods(f.Namespace.Name)
		By("constructing the pod")
		name := "pod" + string(util.NewUUID())
		value := strconv.Itoa(time.Now().Nanosecond())
		podCopy := testingPod(name, value)
		pod := &podCopy
		By("setting up watch")
		selector := labels.SelectorFromSet(labels.Set(map[string]string{"time": value}))
		options := api.ListOptions{LabelSelector: selector}
		pods, err := podClient.List(options)
		if err != nil {
			framework.Failf("Failed to query for pods: %v", err)
		}
		Expect(len(pods.Items)).To(Equal(0))
		options = api.ListOptions{
			LabelSelector:   selector,
			ResourceVersion: pods.ListMeta.ResourceVersion,
		}
		w, err := podClient.Watch(options)
		if err != nil {
			framework.Failf("Failed to set up watch: %v", err)
		}

		By("creating the pod")
		pod, err = podClient.Create(pod)
		if err != nil {
			framework.Failf("Failed to create pod: %v", err)
		}
		// We call defer here in case there is a problem with
		// the test so we can ensure that we clean up after
		// ourselves
		defer podClient.Delete(pod.Name, api.NewDeleteOptions(0))

		By("verifying the pod is in kubernetes")
		options = api.ListOptions{
			LabelSelector:   selector,
			ResourceVersion: pod.ResourceVersion,
		}
		pods, err = podClient.List(options)
		if err != nil {
			framework.Failf("Failed to query for pods: %v", err)
		}
		Expect(len(pods.Items)).To(Equal(1))

		By("verifying pod creation was observed")
		observePodCreation(w)

		// We need to wait for the pod to be scheduled, otherwise the deletion
		// will be carried out immediately rather than gracefully.
		framework.ExpectNoError(f.WaitForPodRunning(pod.Name))

		By("deleting the pod gracefully")
		if err := podClient.Delete(pod.Name, api.NewDeleteOptions(30)); err != nil {
			framework.Failf("Failed to delete pod: %v", err)
		}

		By("verifying pod deletion was observed")
		lastPod := observePodDeletion(w)
		Expect(lastPod.DeletionTimestamp).ToNot(BeNil())
		Expect(lastPod.Spec.TerminationGracePeriodSeconds).ToNot(BeZero())

		options = api.ListOptions{LabelSelector: selector}
		pods, err = podClient.List(options)
		if err != nil {
			Fail(fmt.Sprintf("Failed to list pods to verify deletion: %v", err))
		}
		Expect(len(pods.Items)).To(Equal(0))
	})
})

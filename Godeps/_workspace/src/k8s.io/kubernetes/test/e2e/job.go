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
	"time"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/errors"
	"k8s.io/kubernetes/pkg/apis/batch"
	client "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/kubectl"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/util/wait"
	"k8s.io/kubernetes/test/e2e/framework"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	// How long to wait for a job to finish.
	jobTimeout = 15 * time.Minute

	// Job selector name
	jobSelectorKey = "job"
)

var _ = framework.KubeDescribe("Job", func() {
	f := framework.NewDefaultFramework("job")
	parallelism := int32(2)
	completions := int32(4)
	lotsOfFailures := int32(5) // more than completions

	// Simplest case: all pods succeed promptly
	It("should run a job to completion when tasks succeed", func() {
		By("Creating a job")
		job := newTestJob("succeed", "all-succeed", api.RestartPolicyNever, parallelism, completions)
		job, err := createJob(f.Client, f.Namespace.Name, job)
		Expect(err).NotTo(HaveOccurred())

		By("Ensuring job reaches completions")
		err = waitForJobFinish(f.Client, f.Namespace.Name, job.Name, completions)
		Expect(err).NotTo(HaveOccurred())
	})

	// Pods sometimes fail, but eventually succeed.
	It("should run a job to completion when tasks sometimes fail and are locally restarted", func() {
		By("Creating a job")
		// One failure, then a success, local restarts.
		// We can't use the random failure approach used by the
		// non-local test below, because kubelet will throttle
		// frequently failing containers in a given pod, ramping
		// up to 5 minutes between restarts, making test timeouts
		// due to successive failures too likely with a reasonable
		// test timeout.
		job := newTestJob("failOnce", "fail-once-local", api.RestartPolicyOnFailure, parallelism, completions)
		job, err := createJob(f.Client, f.Namespace.Name, job)
		Expect(err).NotTo(HaveOccurred())

		By("Ensuring job reaches completions")
		err = waitForJobFinish(f.Client, f.Namespace.Name, job.Name, completions)
		Expect(err).NotTo(HaveOccurred())
	})

	// Pods sometimes fail, but eventually succeed, after pod restarts
	It("should run a job to completion when tasks sometimes fail and are not locally restarted", func() {
		By("Creating a job")
		// 50% chance of container success, local restarts.
		// Can't use the failOnce approach because that relies
		// on an emptyDir, which is not preserved across new pods.
		// Worst case analysis: 15 failures, each taking 1 minute to
		// run due to some slowness, 1 in 2^15 chance of happening,
		// causing test flake.  Should be very rare.
		job := newTestJob("randomlySucceedOrFail", "rand-non-local", api.RestartPolicyNever, parallelism, completions)
		job, err := createJob(f.Client, f.Namespace.Name, job)
		Expect(err).NotTo(HaveOccurred())

		By("Ensuring job reaches completions")
		err = waitForJobFinish(f.Client, f.Namespace.Name, job.Name, completions)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should keep restarting failed pods", func() {
		By("Creating a job")
		job := newTestJob("fail", "all-fail", api.RestartPolicyNever, parallelism, completions)
		job, err := createJob(f.Client, f.Namespace.Name, job)
		Expect(err).NotTo(HaveOccurred())

		By("Ensuring job shows many failures")
		err = wait.Poll(framework.Poll, jobTimeout, func() (bool, error) {
			curr, err := f.Client.Extensions().Jobs(f.Namespace.Name).Get(job.Name)
			if err != nil {
				return false, err
			}
			return curr.Status.Failed > lotsOfFailures, nil
		})
	})

	It("should scale a job up", func() {
		startParallelism := int32(1)
		endParallelism := int32(2)
		By("Creating a job")
		job := newTestJob("notTerminate", "scale-up", api.RestartPolicyNever, startParallelism, completions)
		job, err := createJob(f.Client, f.Namespace.Name, job)
		Expect(err).NotTo(HaveOccurred())

		By("Ensuring active pods == startParallelism")
		err = waitForAllPodsRunning(f.Client, f.Namespace.Name, job.Name, startParallelism)
		Expect(err).NotTo(HaveOccurred())

		By("scale job up")
		scaler, err := kubectl.ScalerFor(batch.Kind("Job"), f.Client)
		Expect(err).NotTo(HaveOccurred())
		waitForScale := kubectl.NewRetryParams(5*time.Second, 1*time.Minute)
		waitForReplicas := kubectl.NewRetryParams(5*time.Second, 5*time.Minute)
		scaler.Scale(f.Namespace.Name, job.Name, uint(endParallelism), nil, waitForScale, waitForReplicas)
		Expect(err).NotTo(HaveOccurred())

		By("Ensuring active pods == endParallelism")
		err = waitForAllPodsRunning(f.Client, f.Namespace.Name, job.Name, endParallelism)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should scale a job down", func() {
		startParallelism := int32(2)
		endParallelism := int32(1)
		By("Creating a job")
		job := newTestJob("notTerminate", "scale-down", api.RestartPolicyNever, startParallelism, completions)
		job, err := createJob(f.Client, f.Namespace.Name, job)
		Expect(err).NotTo(HaveOccurred())

		By("Ensuring active pods == startParallelism")
		err = waitForAllPodsRunning(f.Client, f.Namespace.Name, job.Name, startParallelism)
		Expect(err).NotTo(HaveOccurred())

		By("scale job down")
		scaler, err := kubectl.ScalerFor(batch.Kind("Job"), f.Client)
		Expect(err).NotTo(HaveOccurred())
		waitForScale := kubectl.NewRetryParams(5*time.Second, 1*time.Minute)
		waitForReplicas := kubectl.NewRetryParams(5*time.Second, 5*time.Minute)
		err = scaler.Scale(f.Namespace.Name, job.Name, uint(endParallelism), nil, waitForScale, waitForReplicas)
		Expect(err).NotTo(HaveOccurred())

		By("Ensuring active pods == endParallelism")
		err = waitForAllPodsRunning(f.Client, f.Namespace.Name, job.Name, endParallelism)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should delete a job", func() {
		By("Creating a job")
		job := newTestJob("notTerminate", "foo", api.RestartPolicyNever, parallelism, completions)
		job, err := createJob(f.Client, f.Namespace.Name, job)
		Expect(err).NotTo(HaveOccurred())

		By("Ensuring active pods == parallelism")
		err = waitForAllPodsRunning(f.Client, f.Namespace.Name, job.Name, parallelism)
		Expect(err).NotTo(HaveOccurred())

		By("delete a job")
		reaper, err := kubectl.ReaperFor(batch.Kind("Job"), f.Client)
		Expect(err).NotTo(HaveOccurred())
		timeout := 1 * time.Minute
		err = reaper.Stop(f.Namespace.Name, job.Name, timeout, api.NewDeleteOptions(0))
		Expect(err).NotTo(HaveOccurred())

		By("Ensuring job was deleted")
		_, err = f.Client.Extensions().Jobs(f.Namespace.Name).Get(job.Name)
		Expect(err).To(HaveOccurred())
		Expect(errors.IsNotFound(err)).To(BeTrue())
	})

	It("should fail a job", func() {
		By("Creating a job")
		job := newTestJob("notTerminate", "foo", api.RestartPolicyNever, parallelism, completions)
		activeDeadlineSeconds := int64(10)
		job.Spec.ActiveDeadlineSeconds = &activeDeadlineSeconds
		job, err := createJob(f.Client, f.Namespace.Name, job)
		Expect(err).NotTo(HaveOccurred())

		By("Ensuring job was failed")
		err = waitForJobFail(f.Client, f.Namespace.Name, job.Name)
		Expect(err).NotTo(HaveOccurred())
	})
})

// newTestJob returns a job which does one of several testing behaviors.
func newTestJob(behavior, name string, rPol api.RestartPolicy, parallelism, completions int32) *batch.Job {
	job := &batch.Job{
		ObjectMeta: api.ObjectMeta{
			Name: name,
		},
		Spec: batch.JobSpec{
			Parallelism:    &parallelism,
			Completions:    &completions,
			ManualSelector: newBool(true),
			Template: api.PodTemplateSpec{
				ObjectMeta: api.ObjectMeta{
					Labels: map[string]string{jobSelectorKey: name},
				},
				Spec: api.PodSpec{
					RestartPolicy: rPol,
					Volumes: []api.Volume{
						{
							Name: "data",
							VolumeSource: api.VolumeSource{
								EmptyDir: &api.EmptyDirVolumeSource{},
							},
						},
					},
					Containers: []api.Container{
						{
							Name:    "c",
							Image:   "gcr.io/google_containers/busybox:1.24",
							Command: []string{},
							VolumeMounts: []api.VolumeMount{
								{
									MountPath: "/data",
									Name:      "data",
								},
							},
						},
					},
				},
			},
		},
	}
	switch behavior {
	case "notTerminate":
		job.Spec.Template.Spec.Containers[0].Command = []string{"sleep", "1000000"}
	case "fail":
		job.Spec.Template.Spec.Containers[0].Command = []string{"/bin/sh", "-c", "exit 1"}
	case "succeed":
		job.Spec.Template.Spec.Containers[0].Command = []string{"/bin/sh", "-c", "exit 0"}
	case "randomlySucceedOrFail":
		// Bash's $RANDOM generates pseudorandom int in range 0 - 32767.
		// Dividing by 16384 gives roughly 50/50 chance of success.
		job.Spec.Template.Spec.Containers[0].Command = []string{"/bin/sh", "-c", "exit $(( $RANDOM / 16384 ))"}
	case "failOnce":
		// Fail the first the container of the pod is run, and
		// succeed the second time. Checks for file on emptydir.
		// If present, succeed.  If not, create but fail.
		// Note that this cannot be used with RestartNever because
		// it always fails the first time for a pod.
		job.Spec.Template.Spec.Containers[0].Command = []string{"/bin/sh", "-c", "if [[ -r /data/foo ]] ; then exit 0 ; else touch /data/foo ; exit 1 ; fi"}
	}
	return job
}

func createJob(c *client.Client, ns string, job *batch.Job) (*batch.Job, error) {
	return c.Extensions().Jobs(ns).Create(job)
}

func deleteJob(c *client.Client, ns, name string) error {
	return c.Extensions().Jobs(ns).Delete(name, nil)
}

// Wait for all pods to become Running.  Only use when pods will run for a long time, or it will be racy.
func waitForAllPodsRunning(c *client.Client, ns, jobName string, parallelism int32) error {
	label := labels.SelectorFromSet(labels.Set(map[string]string{jobSelectorKey: jobName}))
	return wait.Poll(framework.Poll, jobTimeout, func() (bool, error) {
		options := api.ListOptions{LabelSelector: label}
		pods, err := c.Pods(ns).List(options)
		if err != nil {
			return false, err
		}
		count := int32(0)
		for _, p := range pods.Items {
			if p.Status.Phase == api.PodRunning {
				count++
			}
		}
		return count == parallelism, nil
	})
}

// Wait for job to reach completions.
func waitForJobFinish(c *client.Client, ns, jobName string, completions int32) error {
	return wait.Poll(framework.Poll, jobTimeout, func() (bool, error) {
		curr, err := c.Extensions().Jobs(ns).Get(jobName)
		if err != nil {
			return false, err
		}
		return curr.Status.Succeeded == completions, nil
	})
}

// Wait for job fail.
func waitForJobFail(c *client.Client, ns, jobName string) error {
	return wait.Poll(framework.Poll, jobTimeout, func() (bool, error) {
		curr, err := c.Extensions().Jobs(ns).Get(jobName)
		if err != nil {
			return false, err
		}
		for _, c := range curr.Status.Conditions {
			if c.Type == batch.JobFailed && c.Status == api.ConditionTrue {
				return true, nil
			}
		}
		return false, nil
	})
}

func newBool(val bool) *bool {
	p := new(bool)
	*p = val
	return p
}

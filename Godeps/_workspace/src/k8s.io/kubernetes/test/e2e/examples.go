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
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"k8s.io/kubernetes/pkg/api"
	client "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/test/e2e/framework"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	serverStartTimeout = framework.PodStartTimeout + 3*time.Minute
)

var _ = framework.KubeDescribe("[Feature:Example]", func() {
	f := framework.NewDefaultFramework("examples")
	// Customized ForEach wrapper for this test.
	forEachPod := func(selectorKey string, selectorValue string, fn func(api.Pod)) {
		f.NewClusterVerification(
			framework.PodStateVerification{
				Selectors:   map[string]string{selectorKey: selectorValue},
				ValidPhases: []api.PodPhase{api.PodRunning},
			}).ForEach(fn)
	}
	var c *client.Client
	var ns string
	BeforeEach(func() {
		c = f.Client
		ns = f.Namespace.Name
	})

	framework.KubeDescribe("Redis", func() {
		It("should create and stop redis servers", func() {
			mkpath := func(file string) string {
				return filepath.Join(framework.TestContext.RepoRoot, "examples/redis", file)
			}
			bootstrapYaml := mkpath("redis-master.yaml")
			sentinelServiceYaml := mkpath("redis-sentinel-service.yaml")
			sentinelControllerYaml := mkpath("redis-sentinel-controller.yaml")
			controllerYaml := mkpath("redis-controller.yaml")

			bootstrapPodName := "redis-master"
			redisRC := "redis"
			sentinelRC := "redis-sentinel"
			nsFlag := fmt.Sprintf("--namespace=%v", ns)
			expectedOnServer := "The server is now ready to accept connections"
			expectedOnSentinel := "+monitor master"

			By("starting redis bootstrap")
			framework.RunKubectlOrDie("create", "-f", bootstrapYaml, nsFlag)
			err := framework.WaitForPodRunningInNamespace(c, bootstrapPodName, ns)
			Expect(err).NotTo(HaveOccurred())

			_, err = framework.LookForStringInLog(ns, bootstrapPodName, "master", expectedOnServer, serverStartTimeout)
			Expect(err).NotTo(HaveOccurred())
			_, err = framework.LookForStringInLog(ns, bootstrapPodName, "sentinel", expectedOnSentinel, serverStartTimeout)
			Expect(err).NotTo(HaveOccurred())

			By("setting up services and controllers")
			framework.RunKubectlOrDie("create", "-f", sentinelServiceYaml, nsFlag)
			framework.RunKubectlOrDie("create", "-f", sentinelControllerYaml, nsFlag)
			framework.RunKubectlOrDie("create", "-f", controllerYaml, nsFlag)

			By("scaling up the deployment")
			framework.RunKubectlOrDie("scale", "rc", redisRC, "--replicas=3", nsFlag)
			framework.RunKubectlOrDie("scale", "rc", sentinelRC, "--replicas=3", nsFlag)

			By("checking up the services")
			checkAllLogs := func() {
				forEachPod("name", "redis", func(pod api.Pod) {
					if pod.Name != bootstrapPodName {
						_, err := framework.LookForStringInLog(ns, pod.Name, "redis", expectedOnServer, serverStartTimeout)
						Expect(err).NotTo(HaveOccurred())
					}
				})
				forEachPod("name", "redis-sentinel", func(pod api.Pod) {
					if pod.Name != bootstrapPodName {
						_, err := framework.LookForStringInLog(ns, pod.Name, "sentinel", expectedOnSentinel, serverStartTimeout)
						Expect(err).NotTo(HaveOccurred())
					}
				})
			}
			checkAllLogs()

			By("turning down bootstrap")
			framework.RunKubectlOrDie("delete", "-f", bootstrapYaml, nsFlag)
			err = framework.WaitForRCPodToDisappear(c, ns, redisRC, bootstrapPodName)
			Expect(err).NotTo(HaveOccurred())
			By("waiting for the new master election")
			checkAllLogs()
		})
	})

	framework.KubeDescribe("Celery-RabbitMQ", func() {
		It("should create and stop celery+rabbitmq servers", func() {
			mkpath := func(file string) string {
				return filepath.Join(framework.TestContext.RepoRoot, "examples", "celery-rabbitmq", file)
			}
			rabbitmqServiceYaml := mkpath("rabbitmq-service.yaml")
			rabbitmqControllerYaml := mkpath("rabbitmq-controller.yaml")
			celeryControllerYaml := mkpath("celery-controller.yaml")
			flowerControllerYaml := mkpath("flower-controller.yaml")
			flowerServiceYaml := mkpath("flower-service.yaml")
			nsFlag := fmt.Sprintf("--namespace=%v", ns)

			By("starting rabbitmq")
			framework.RunKubectlOrDie("create", "-f", rabbitmqServiceYaml, nsFlag)
			framework.RunKubectlOrDie("create", "-f", rabbitmqControllerYaml, nsFlag)
			forEachPod("component", "rabbitmq", func(pod api.Pod) {
				_, err := framework.LookForStringInLog(ns, pod.Name, "rabbitmq", "Server startup complete", serverStartTimeout)
				Expect(err).NotTo(HaveOccurred())
			})
			err := framework.WaitForEndpoint(c, ns, "rabbitmq-service")
			Expect(err).NotTo(HaveOccurred())

			By("starting celery")
			framework.RunKubectlOrDie("create", "-f", celeryControllerYaml, nsFlag)
			forEachPod("component", "celery", func(pod api.Pod) {
				_, err := framework.LookForStringInFile(ns, pod.Name, "celery", "/data/celery.log", " ready.", serverStartTimeout)
				Expect(err).NotTo(HaveOccurred())
			})

			By("starting flower")
			framework.RunKubectlOrDie("create", "-f", flowerServiceYaml, nsFlag)
			framework.RunKubectlOrDie("create", "-f", flowerControllerYaml, nsFlag)
			forEachPod("component", "flower", func(pod api.Pod) {

			})
			forEachPod("component", "flower", func(pod api.Pod) {
				content, err := makeHttpRequestToService(c, ns, "flower-service", "/", framework.EndpointRegisterTimeout)
				Expect(err).NotTo(HaveOccurred())
				if !strings.Contains(content, "<title>Celery Flower</title>") {
					framework.Failf("Flower HTTP request failed")
				}
			})
		})
	})

	framework.KubeDescribe("Spark", func() {
		It("should start spark master, driver and workers", func() {
			mkpath := func(file string) string {
				return filepath.Join(framework.TestContext.RepoRoot, "examples", "spark", file)
			}

			// TODO: Add Zepplin and Web UI to this example.
			serviceYaml := mkpath("spark-master-service.yaml")
			masterYaml := mkpath("spark-master-controller.yaml")
			workerControllerYaml := mkpath("spark-worker-controller.yaml")
			nsFlag := fmt.Sprintf("--namespace=%v", ns)

			master := func() {
				By("starting master")
				framework.RunKubectlOrDie("create", "-f", serviceYaml, nsFlag)
				framework.RunKubectlOrDie("create", "-f", masterYaml, nsFlag)

				framework.Logf("Now polling for Master startup...")

				// Only one master pod: But its a natural way to look up pod names.
				forEachPod("component", "spark-master", func(pod api.Pod) {
					framework.Logf("Now waiting for master to startup in %v", pod.Name)
					_, err := framework.LookForStringInLog(ns, pod.Name, "spark-master", "Starting Spark master at", serverStartTimeout)
					Expect(err).NotTo(HaveOccurred())
				})

				By("waiting for master endpoint")
				err := framework.WaitForEndpoint(c, ns, "spark-master")
				Expect(err).NotTo(HaveOccurred())
				forEachPod("component", "spark-master", func(pod api.Pod) {
					_, maErr := framework.LookForStringInLog(f.Namespace.Name, pod.Name, "spark-master", "Starting Spark master at", serverStartTimeout)
					if maErr != nil {
						framework.Failf("Didn't find target string. error:", maErr)
					}
				})
			}
			worker := func() {
				By("starting workers")
				framework.Logf("Now starting Workers")
				framework.RunKubectlOrDie("create", "-f", workerControllerYaml, nsFlag)

				// For now, scaling is orthogonal to the core test.
				// framework.ScaleRC(c, ns, "spark-worker-controller", 2, true)

				framework.Logf("Now polling for worker startup...")
				// ScaleRC(c, ns, "spark-worker-controller", 2, true)
				framework.Logf("Now polling for worker startup...")
				forEachPod("component", "spark-worker",
					func(pod api.Pod) {
						_, slaveErr := framework.LookForStringInLog(ns, pod.Name, "spark-worker", "Successfully registered with master", serverStartTimeout)
						Expect(slaveErr).NotTo(HaveOccurred())
					})
			}
			// Run the worker verification after we turn up the master.
			defer worker()
			master()
		})
	})

	framework.KubeDescribe("Cassandra", func() {
		It("should create and scale cassandra", func() {
			mkpath := func(file string) string {
				return filepath.Join(framework.TestContext.RepoRoot, "examples", "cassandra", file)
			}
			serviceYaml := mkpath("cassandra-service.yaml")
			controllerYaml := mkpath("cassandra-controller.yaml")
			nsFlag := fmt.Sprintf("--namespace=%v", ns)

			By("Starting the cassandra service")
			framework.RunKubectlOrDie("create", "-f", serviceYaml, nsFlag)

			framework.Logf("wait for service")
			err := framework.WaitForEndpoint(c, ns, "cassandra")
			Expect(err).NotTo(HaveOccurred())

			// Create an RC with n nodes in it.  Each node will then be verified.
			By("Creating a Cassandra RC")
			framework.RunKubectlOrDie("create", "-f", controllerYaml, nsFlag)
			forEachPod("app", "cassandra", func(pod api.Pod) {
				framework.Logf("Verifying pod %v ", pod.Name)
				_, err = framework.LookForStringInLog(ns, pod.Name, "cassandra", "Listening for thrift clients", serverStartTimeout)
				Expect(err).NotTo(HaveOccurred())
				_, err = framework.LookForStringInLog(ns, pod.Name, "cassandra", "Handshaking version", serverStartTimeout)
				Expect(err).NotTo(HaveOccurred())
			})

			By("Finding each node in the nodetool status lines")
			output := framework.RunKubectlOrDie("exec", "cassandra", nsFlag, "--", "nodetool", "status")
			forEachPod("app", "cassandra", func(pod api.Pod) {
				if !strings.Contains(output, pod.Status.PodIP) {
					framework.Failf("Pod ip %s not found in nodetool status", pod.Status.PodIP)
				}
			})
		})
	})

	framework.KubeDescribe("Storm", func() {
		It("should create and stop Zookeeper, Nimbus and Storm worker servers", func() {
			mkpath := func(file string) string {
				return filepath.Join(framework.TestContext.RepoRoot, "examples", "storm", file)
			}
			zookeeperServiceJson := mkpath("zookeeper-service.json")
			zookeeperPodJson := mkpath("zookeeper.json")
			nimbusServiceJson := mkpath("storm-nimbus-service.json")
			nimbusPodJson := mkpath("storm-nimbus.json")
			workerControllerJson := mkpath("storm-worker-controller.json")
			nsFlag := fmt.Sprintf("--namespace=%v", ns)
			zookeeperPod := "zookeeper"

			By("starting Zookeeper")
			framework.RunKubectlOrDie("create", "-f", zookeeperPodJson, nsFlag)
			framework.RunKubectlOrDie("create", "-f", zookeeperServiceJson, nsFlag)
			err := framework.WaitForPodRunningInNamespace(c, zookeeperPod, ns)
			Expect(err).NotTo(HaveOccurred())

			By("checking if zookeeper is up and running")
			_, err = framework.LookForStringInLog(ns, zookeeperPod, "zookeeper", "binding to port", serverStartTimeout)
			Expect(err).NotTo(HaveOccurred())
			err = framework.WaitForEndpoint(c, ns, "zookeeper")
			Expect(err).NotTo(HaveOccurred())

			By("starting Nimbus")
			framework.RunKubectlOrDie("create", "-f", nimbusPodJson, nsFlag)
			framework.RunKubectlOrDie("create", "-f", nimbusServiceJson, nsFlag)
			err = framework.WaitForPodRunningInNamespace(c, "nimbus", ns)
			Expect(err).NotTo(HaveOccurred())

			err = framework.WaitForEndpoint(c, ns, "nimbus")
			Expect(err).NotTo(HaveOccurred())

			By("starting workers")
			framework.RunKubectlOrDie("create", "-f", workerControllerJson, nsFlag)
			forEachPod("name", "storm-worker", func(pod api.Pod) {
				//do nothing, just wait for the pod to be running
			})
			// TODO: Add logging configuration to nimbus & workers images and then
			// look for a string instead of sleeping.
			time.Sleep(20 * time.Second)

			By("checking if there are established connections to Zookeeper")
			_, err = framework.LookForStringInLog(ns, zookeeperPod, "zookeeper", "Established session", serverStartTimeout)
			Expect(err).NotTo(HaveOccurred())

			By("checking if Nimbus responds to requests")
			framework.LookForString("No topologies running.", time.Minute, func() string {
				return framework.RunKubectlOrDie("exec", "nimbus", nsFlag, "--", "bin/storm", "list")
			})
		})
	})

	framework.KubeDescribe("Liveness", func() {
		It("liveness pods should be automatically restarted", func() {
			mkpath := func(file string) string {
				return filepath.Join(framework.TestContext.RepoRoot, "docs", "user-guide", "liveness", file)
			}
			execYaml := mkpath("exec-liveness.yaml")
			httpYaml := mkpath("http-liveness.yaml")
			nsFlag := fmt.Sprintf("--namespace=%v", ns)

			framework.RunKubectlOrDie("create", "-f", execYaml, nsFlag)
			framework.RunKubectlOrDie("create", "-f", httpYaml, nsFlag)

			// Since both containers start rapidly, we can easily run this test in parallel.
			var wg sync.WaitGroup
			passed := true
			checkRestart := func(podName string, timeout time.Duration) {
				err := framework.WaitForPodRunningInNamespace(c, podName, ns)
				Expect(err).NotTo(HaveOccurred())
				for t := time.Now(); time.Since(t) < timeout; time.Sleep(framework.Poll) {
					pod, err := c.Pods(ns).Get(podName)
					framework.ExpectNoError(err, fmt.Sprintf("getting pod %s", podName))
					stat := api.GetExistingContainerStatus(pod.Status.ContainerStatuses, podName)
					framework.Logf("Pod: %s, restart count:%d", stat.Name, stat.RestartCount)
					if stat.RestartCount > 0 {
						framework.Logf("Saw %v restart, succeeded...", podName)
						wg.Done()
						return
					}
				}
				framework.Logf("Failed waiting for %v restart! ", podName)
				passed = false
				wg.Done()
			}

			By("Check restarts")

			// Start the "actual test", and wait for both pods to complete.
			// If 2 fail: Something is broken with the test (or maybe even with liveness).
			// If 1 fails: Its probably just an error in the examples/ files themselves.
			wg.Add(2)
			for _, c := range []string{"liveness-http", "liveness-exec"} {
				go checkRestart(c, 2*time.Minute)
			}
			wg.Wait()
			if !passed {
				framework.Failf("At least one liveness example failed.  See the logs above.")
			}
		})
	})

	framework.KubeDescribe("Secret", func() {
		It("should create a pod that reads a secret", func() {
			mkpath := func(file string) string {
				return filepath.Join(framework.TestContext.RepoRoot, "docs", "user-guide", "secrets", file)
			}
			secretYaml := mkpath("secret.yaml")
			podYaml := mkpath("secret-pod.yaml")
			nsFlag := fmt.Sprintf("--namespace=%v", ns)
			podName := "secret-test-pod"

			By("creating secret and pod")
			framework.RunKubectlOrDie("create", "-f", secretYaml, nsFlag)
			framework.RunKubectlOrDie("create", "-f", podYaml, nsFlag)
			err := framework.WaitForPodNoLongerRunningInNamespace(c, podName, ns)
			Expect(err).NotTo(HaveOccurred())

			By("checking if secret was read correctly")
			_, err = framework.LookForStringInLog(ns, "secret-test-pod", "test-container", "value-1", serverStartTimeout)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	framework.KubeDescribe("Downward API", func() {
		It("should create a pod that prints his name and namespace", func() {
			mkpath := func(file string) string {
				return filepath.Join(framework.TestContext.RepoRoot, "docs", "user-guide", "downward-api", file)
			}
			podYaml := mkpath("dapi-pod.yaml")
			nsFlag := fmt.Sprintf("--namespace=%v", ns)
			podName := "dapi-test-pod"

			By("creating the pod")
			framework.RunKubectlOrDie("create", "-f", podYaml, nsFlag)
			err := framework.WaitForPodNoLongerRunningInNamespace(c, podName, ns)
			Expect(err).NotTo(HaveOccurred())

			By("checking if name and namespace were passed correctly")
			_, err = framework.LookForStringInLog(ns, podName, "test-container", fmt.Sprintf("MY_POD_NAMESPACE=%v", ns), serverStartTimeout)
			Expect(err).NotTo(HaveOccurred())
			_, err = framework.LookForStringInLog(ns, podName, "test-container", fmt.Sprintf("MY_POD_NAME=%v", podName), serverStartTimeout)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	framework.KubeDescribe("RethinkDB", func() {
		It("should create and stop rethinkdb servers", func() {
			mkpath := func(file string) string {
				return filepath.Join(framework.TestContext.RepoRoot, "examples", "rethinkdb", file)
			}
			driverServiceYaml := mkpath("driver-service.yaml")
			rethinkDbControllerYaml := mkpath("rc.yaml")
			adminPodYaml := mkpath("admin-pod.yaml")
			adminServiceYaml := mkpath("admin-service.yaml")
			nsFlag := fmt.Sprintf("--namespace=%v", ns)

			By("starting rethinkdb")
			framework.RunKubectlOrDie("create", "-f", driverServiceYaml, nsFlag)
			framework.RunKubectlOrDie("create", "-f", rethinkDbControllerYaml, nsFlag)
			checkDbInstances := func() {
				forEachPod("db", "rethinkdb", func(pod api.Pod) {
					_, err := framework.LookForStringInLog(ns, pod.Name, "rethinkdb", "Server ready", serverStartTimeout)
					Expect(err).NotTo(HaveOccurred())
				})
			}
			checkDbInstances()
			err := framework.WaitForEndpoint(c, ns, "rethinkdb-driver")
			Expect(err).NotTo(HaveOccurred())

			By("scaling rethinkdb")
			framework.ScaleRC(c, ns, "rethinkdb-rc", 2, true)
			checkDbInstances()

			By("starting admin")
			framework.RunKubectlOrDie("create", "-f", adminServiceYaml, nsFlag)
			framework.RunKubectlOrDie("create", "-f", adminPodYaml, nsFlag)
			err = framework.WaitForPodRunningInNamespace(c, "rethinkdb-admin", ns)
			Expect(err).NotTo(HaveOccurred())
			checkDbInstances()
			content, err := makeHttpRequestToService(c, ns, "rethinkdb-admin", "/", framework.EndpointRegisterTimeout)
			Expect(err).NotTo(HaveOccurred())
			if !strings.Contains(content, "<title>RethinkDB Administration Console</title>") {
				framework.Failf("RethinkDB console is not running")
			}
		})
	})

	framework.KubeDescribe("Hazelcast", func() {
		It("should create and scale hazelcast", func() {
			mkpath := func(file string) string {
				return filepath.Join(framework.TestContext.RepoRoot, "examples", "hazelcast", file)
			}
			serviceYaml := mkpath("hazelcast-service.yaml")
			controllerYaml := mkpath("hazelcast-controller.yaml")
			nsFlag := fmt.Sprintf("--namespace=%v", ns)

			By("starting hazelcast")
			framework.RunKubectlOrDie("create", "-f", serviceYaml, nsFlag)
			framework.RunKubectlOrDie("create", "-f", controllerYaml, nsFlag)
			forEachPod("name", "hazelcast", func(pod api.Pod) {
				_, err := framework.LookForStringInLog(ns, pod.Name, "hazelcast", "Members [1]", serverStartTimeout)
				Expect(err).NotTo(HaveOccurred())
				_, err = framework.LookForStringInLog(ns, pod.Name, "hazelcast", "is STARTED", serverStartTimeout)
				Expect(err).NotTo(HaveOccurred())
			})

			err := framework.WaitForEndpoint(c, ns, "hazelcast")
			Expect(err).NotTo(HaveOccurred())

			By("scaling hazelcast")
			framework.ScaleRC(c, ns, "hazelcast", 2, true)
			forEachPod("name", "hazelcast", func(pod api.Pod) {
				_, err := framework.LookForStringInLog(ns, pod.Name, "hazelcast", "Members [2]", serverStartTimeout)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})

func makeHttpRequestToService(c *client.Client, ns, service, path string, timeout time.Duration) (string, error) {
	var result []byte
	var err error
	for t := time.Now(); time.Since(t) < timeout; time.Sleep(framework.Poll) {
		proxyRequest, errProxy := framework.GetServicesProxyRequest(c, c.Get())
		if errProxy != nil {
			break
		}
		result, err = proxyRequest.Namespace(ns).
			Name(service).
			Suffix(path).
			Do().
			Raw()
		if err != nil {
			break
		}
	}
	return string(result), err
}

// pass enough context with the 'old' parameter so that it replaces what your really intended.
func prepareResourceWithReplacedString(inputFile, old, new string) string {
	f, err := os.Open(inputFile)
	Expect(err).NotTo(HaveOccurred())
	defer f.Close()
	data, err := ioutil.ReadAll(f)
	Expect(err).NotTo(HaveOccurred())
	podYaml := strings.Replace(string(data), old, new, 1)
	return podYaml
}

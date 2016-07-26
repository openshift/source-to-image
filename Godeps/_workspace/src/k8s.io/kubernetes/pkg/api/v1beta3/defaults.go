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

package v1beta3

import (
	"strings"

	"github.com/golang/glog"

	"k8s.io/kubernetes/pkg/runtime"
	"k8s.io/kubernetes/pkg/util"
	"k8s.io/kubernetes/pkg/util/intstr"
)

func addDefaultingFuncs(scheme *runtime.Scheme) {
	scheme.AddDefaultingFuncs(
		func(obj *ReplicationController) {
			var labels map[string]string
			if obj.Spec.Template != nil {
				labels = obj.Spec.Template.Labels
			}
			// TODO: support templates defined elsewhere when we support them in the API
			if labels != nil {
				if len(obj.Spec.Selector) == 0 {
					obj.Spec.Selector = labels
				}
				if len(obj.Labels) == 0 {
					obj.Labels = labels
				}
			}
			if obj.Spec.Replicas == nil {
				obj.Spec.Replicas = new(int)
				*obj.Spec.Replicas = 0
			}
		},
		func(obj *Volume) {
			if util.AllPtrFieldsNil(&obj.VolumeSource) {
				obj.VolumeSource = VolumeSource{
					EmptyDir: &EmptyDirVolumeSource{},
				}
			}
		},
		func(obj *ContainerPort) {
			if obj.Protocol == "" {
				obj.Protocol = ProtocolTCP
			}
		},
		func(obj *Container) {
			if obj.ImagePullPolicy == "" {
				// TODO(dchen1107): Move ParseImageName code to pkg/util
				parts := strings.Split(obj.Image, ":")
				// Check image tag
				if parts[len(parts)-1] == "latest" {
					obj.ImagePullPolicy = PullAlways
				} else {
					obj.ImagePullPolicy = PullIfNotPresent
				}
			}
			if obj.TerminationMessagePath == "" {
				obj.TerminationMessagePath = TerminationMessagePathDefault
			}
			defaultSecurityContext(obj)
		},
		func(obj *ServiceSpec) {
			if obj.SessionAffinity == "" {
				obj.SessionAffinity = ServiceAffinityNone
			}
			if obj.Type == "" {
				if obj.CreateExternalLoadBalancer {
					obj.Type = ServiceTypeLoadBalancer
				} else {
					obj.Type = ServiceTypeClusterIP
				}
			} else if obj.Type == ServiceTypeLoadBalancer {
				obj.CreateExternalLoadBalancer = true
			}
			for i := range obj.Ports {
				sp := &obj.Ports[i]
				if sp.Protocol == "" {
					sp.Protocol = ProtocolTCP
				}
				if sp.TargetPort == intstr.FromInt(0) || sp.TargetPort == intstr.FromString("") {
					sp.TargetPort = intstr.FromInt(sp.Port)
				}
			}
		},
		func(obj *PodSpec) {
			if obj.DNSPolicy == "" {
				obj.DNSPolicy = DNSClusterFirst
			}
			if obj.RestartPolicy == "" {
				obj.RestartPolicy = RestartPolicyAlways
			}
			if obj.HostNetwork {
				defaultHostNetworkPorts(&obj.Containers)
			}
		},
		func(obj *Probe) {
			if obj.TimeoutSeconds == 0 {
				obj.TimeoutSeconds = 1
			}
		},
		func(obj *Secret) {
			if obj.Type == "" {
				obj.Type = SecretTypeOpaque
			}
		},
		func(obj *PersistentVolume) {
			if obj.Status.Phase == "" {
				obj.Status.Phase = VolumePending
			}
			if obj.Spec.PersistentVolumeReclaimPolicy == "" {
				obj.Spec.PersistentVolumeReclaimPolicy = PersistentVolumeReclaimRetain
			}
		},
		func(obj *PersistentVolumeClaim) {
			if obj.Status.Phase == "" {
				obj.Status.Phase = ClaimPending
			}
		},
		func(obj *Endpoints) {
			for i := range obj.Subsets {
				ss := &obj.Subsets[i]
				for i := range ss.Ports {
					ep := &ss.Ports[i]
					if ep.Protocol == "" {
						ep.Protocol = ProtocolTCP
					}
				}
			}
		},
		func(obj *HTTPGetAction) {
			if obj.Path == "" {
				obj.Path = "/"
			}
			if obj.Scheme == "" {
				obj.Scheme = URISchemeHTTP
			}
		},
		func(obj *NamespaceStatus) {
			if obj.Phase == "" {
				obj.Phase = NamespaceActive
			}
		},
		func(obj *Node) {
			if obj.Spec.ExternalID == "" {
				obj.Spec.ExternalID = obj.Name
			}
		},
		func(obj *ObjectFieldSelector) {
			if obj.APIVersion == "" {
				obj.APIVersion = "v1beta3"
			}
		},
		func(obj *SecurityContextConstraints) {
			defaultSecurityContextConstraints(obj)
		},
	)
}

// With host networking default all container ports to host ports.
func defaultHostNetworkPorts(containers *[]Container) {
	for i := range *containers {
		for j := range (*containers)[i].Ports {
			if (*containers)[i].Ports[j].HostPort == 0 {
				(*containers)[i].Ports[j].HostPort = (*containers)[i].Ports[j].ContainerPort
			}
		}
	}
}

// defaultSecurityContext performs the downward and upward merges of a pod definition
func defaultSecurityContext(container *Container) {
	if container.SecurityContext == nil {
		if (len(container.Capabilities.Add) == 0) && (len(container.Capabilities.Drop) == 0) && (container.Privileged == false) {
			return
		}
		glog.V(5).Infof("creating security context for container %s", container.Name)
		container.SecurityContext = &SecurityContext{}
	}
	// if there are no capabilities defined on the SecurityContext then copy the container settings
	if container.SecurityContext.Capabilities == nil {
		container.SecurityContext.Capabilities = &container.Capabilities
	} else {
		// if there are capabilities defined on the security context and the container setting is
		// empty then assume that it was left off the pod definition and ensure that the container
		// settings match the security context settings (checked by the convert functions).  If
		// there are settings in both then don't touch it, the converter will error if they don't
		// match
		if len(container.Capabilities.Add) == 0 {
			container.Capabilities.Add = container.SecurityContext.Capabilities.Add
		}
		if len(container.Capabilities.Drop) == 0 {
			container.Capabilities.Drop = container.SecurityContext.Capabilities.Drop
		}
	}
	// if there are no privileged settings on the security context then copy the container settings
	if container.SecurityContext.Privileged == nil {
		container.SecurityContext.Privileged = &container.Privileged
	} else {
		// we don't have a good way to know if container.Privileged was set or just defaulted to false
		// so the best we can do here is check if the securityContext is set to true and the
		// container is set to false and assume that the Privileged field was left off the container
		// definition and not an intentional mismatch
		if *container.SecurityContext.Privileged && !container.Privileged {
			container.Privileged = *container.SecurityContext.Privileged
		}
	}
}

// Default SCCs for new fields.  FSGroup and SupplementalGroups are
// set to the RunAsAny strategy if they are unset on the scc.
func defaultSecurityContextConstraints(scc *SecurityContextConstraints) {
	if len(scc.FSGroup.Type) == 0 {
		scc.FSGroup.Type = FSGroupStrategyRunAsAny
	}
	if len(scc.SupplementalGroups.Type) == 0 {
		scc.SupplementalGroups.Type = SupplementalGroupsStrategyRunAsAny
	}
}

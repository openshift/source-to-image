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
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/runtime"
)

// GroupName is the group name use in this package
const GroupName = ""

// SchemeGroupVersion is group version used to register these objects
var SchemeGroupVersion = unversioned.GroupVersion{Group: GroupName, Version: "v1beta3"}

func AddToScheme(scheme *runtime.Scheme) {
	// Add the API to Scheme.
	addKnownTypes(scheme)
	addConversionFuncs(scheme)
	addDefaultingFuncs(scheme)
}

// Adds the list of known types to api.Scheme.
func addKnownTypes(scheme *runtime.Scheme) {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&Pod{},
		&PodList{},
		&PodStatusResult{},
		&PodTemplate{},
		&PodTemplateList{},
		&ReplicationController{},
		&ReplicationControllerList{},
		&Service{},
		&ServiceList{},
		&Endpoints{},
		&EndpointsList{},
		&Node{},
		&NodeList{},
		&Binding{},
		&Event{},
		&EventList{},
		&List{},
		&LimitRange{},
		&LimitRangeList{},
		&ResourceQuota{},
		&ResourceQuotaList{},
		&Namespace{},
		&NamespaceList{},
		&Secret{},
		&SecretList{},
		&ServiceAccount{},
		&ServiceAccountList{},
		&PersistentVolume{},
		&PersistentVolumeList{},
		&PersistentVolumeClaim{},
		&PersistentVolumeClaimList{},
		&DeleteOptions{},
		&ListOptions{},
		&PodAttachOptions{},
		&PodLogOptions{},
		&PodExecOptions{},
		&PodProxyOptions{},
		&ComponentStatus{},
		&ComponentStatusList{},
		&SerializedReference{},
		&RangeAllocation{},
		&SecurityContextConstraints{},
		&SecurityContextConstraintsList{},
	)
	// Legacy names are supported
	api.Scheme.AddKnownTypeWithName(SchemeGroupVersion.WithKind("Minion"), &Node{})
	api.Scheme.AddKnownTypeWithName(SchemeGroupVersion.WithKind("MinionList"), &NodeList{})

	// Add common types
	api.Scheme.AddKnownTypes(SchemeGroupVersion, &unversioned.Status{})
}

// SetGroupVersionKind satisfies the ObjectKind interface for all objects that embed TypeMeta
func (obj *TypeMeta) SetGroupVersionKind(gvk *unversioned.GroupVersionKind) {
	obj.APIVersion, obj.Kind = gvk.ToAPIVersionAndKind()
}

// GroupVersionKind satisfies the ObjectKind interface for all objects that embed TypeMeta
func (obj *TypeMeta) GroupVersionKind() *unversioned.GroupVersionKind {
	return unversioned.FromAPIVersionAndKind(obj.APIVersion, obj.Kind)
}

func (obj *Pod) GetObjectKind() unversioned.ObjectKind                       { return &obj.TypeMeta }
func (obj *PodList) GetObjectKind() unversioned.ObjectKind                   { return &obj.TypeMeta }
func (obj *PodStatusResult) GetObjectKind() unversioned.ObjectKind           { return &obj.TypeMeta }
func (obj *PodTemplate) GetObjectKind() unversioned.ObjectKind               { return &obj.TypeMeta }
func (obj *PodTemplateList) GetObjectKind() unversioned.ObjectKind           { return &obj.TypeMeta }
func (obj *ReplicationController) GetObjectKind() unversioned.ObjectKind     { return &obj.TypeMeta }
func (obj *ReplicationControllerList) GetObjectKind() unversioned.ObjectKind { return &obj.TypeMeta }
func (obj *Service) GetObjectKind() unversioned.ObjectKind                   { return &obj.TypeMeta }
func (obj *ServiceList) GetObjectKind() unversioned.ObjectKind               { return &obj.TypeMeta }
func (obj *Endpoints) GetObjectKind() unversioned.ObjectKind                 { return &obj.TypeMeta }
func (obj *EndpointsList) GetObjectKind() unversioned.ObjectKind             { return &obj.TypeMeta }
func (obj *Node) GetObjectKind() unversioned.ObjectKind                      { return &obj.TypeMeta }
func (obj *NodeList) GetObjectKind() unversioned.ObjectKind                  { return &obj.TypeMeta }
func (obj *Binding) GetObjectKind() unversioned.ObjectKind                   { return &obj.TypeMeta }
func (obj *Event) GetObjectKind() unversioned.ObjectKind                     { return &obj.TypeMeta }
func (obj *EventList) GetObjectKind() unversioned.ObjectKind                 { return &obj.TypeMeta }
func (obj *List) GetObjectKind() unversioned.ObjectKind                      { return &obj.TypeMeta }
func (obj *LimitRange) GetObjectKind() unversioned.ObjectKind                { return &obj.TypeMeta }
func (obj *LimitRangeList) GetObjectKind() unversioned.ObjectKind            { return &obj.TypeMeta }
func (obj *ResourceQuota) GetObjectKind() unversioned.ObjectKind             { return &obj.TypeMeta }
func (obj *ResourceQuotaList) GetObjectKind() unversioned.ObjectKind         { return &obj.TypeMeta }
func (obj *Namespace) GetObjectKind() unversioned.ObjectKind                 { return &obj.TypeMeta }
func (obj *NamespaceList) GetObjectKind() unversioned.ObjectKind             { return &obj.TypeMeta }
func (obj *Secret) GetObjectKind() unversioned.ObjectKind                    { return &obj.TypeMeta }
func (obj *SecretList) GetObjectKind() unversioned.ObjectKind                { return &obj.TypeMeta }
func (obj *ServiceAccount) GetObjectKind() unversioned.ObjectKind            { return &obj.TypeMeta }
func (obj *ServiceAccountList) GetObjectKind() unversioned.ObjectKind        { return &obj.TypeMeta }
func (obj *PersistentVolume) GetObjectKind() unversioned.ObjectKind          { return &obj.TypeMeta }
func (obj *PersistentVolumeList) GetObjectKind() unversioned.ObjectKind      { return &obj.TypeMeta }
func (obj *PersistentVolumeClaim) GetObjectKind() unversioned.ObjectKind     { return &obj.TypeMeta }
func (obj *PersistentVolumeClaimList) GetObjectKind() unversioned.ObjectKind { return &obj.TypeMeta }
func (obj *DeleteOptions) GetObjectKind() unversioned.ObjectKind             { return &obj.TypeMeta }
func (obj *ListOptions) GetObjectKind() unversioned.ObjectKind               { return &obj.TypeMeta }
func (obj *PodAttachOptions) GetObjectKind() unversioned.ObjectKind          { return &obj.TypeMeta }
func (obj *PodLogOptions) GetObjectKind() unversioned.ObjectKind             { return &obj.TypeMeta }
func (obj *PodExecOptions) GetObjectKind() unversioned.ObjectKind            { return &obj.TypeMeta }
func (obj *PodProxyOptions) GetObjectKind() unversioned.ObjectKind           { return &obj.TypeMeta }
func (obj *ComponentStatus) GetObjectKind() unversioned.ObjectKind           { return &obj.TypeMeta }
func (obj *ComponentStatusList) GetObjectKind() unversioned.ObjectKind       { return &obj.TypeMeta }
func (obj *SerializedReference) GetObjectKind() unversioned.ObjectKind       { return &obj.TypeMeta }
func (obj *RangeAllocation) GetObjectKind() unversioned.ObjectKind           { return &obj.TypeMeta }

func (obj *SecurityContextConstraints) GetObjectKind() unversioned.ObjectKind { return &obj.TypeMeta }
func (obj *SecurityContextConstraintsList) GetObjectKind() unversioned.ObjectKind {
	return &obj.TypeMeta
}

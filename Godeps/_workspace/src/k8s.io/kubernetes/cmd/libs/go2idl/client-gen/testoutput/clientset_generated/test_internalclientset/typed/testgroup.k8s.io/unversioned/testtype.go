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

package unversioned

import (
	testgroup_k8s_io "k8s.io/kubernetes/cmd/libs/go2idl/client-gen/test_apis/testgroup.k8s.io"
	api "k8s.io/kubernetes/pkg/api"
	watch "k8s.io/kubernetes/pkg/watch"
)

// TestTypesGetter has a method to return a TestTypeInterface.
// A group's client should implement this interface.
type TestTypesGetter interface {
	TestTypes(namespace string) TestTypeInterface
}

// TestTypeInterface has methods to work with TestType resources.
type TestTypeInterface interface {
	Create(*testgroup_k8s_io.TestType) (*testgroup_k8s_io.TestType, error)
	Update(*testgroup_k8s_io.TestType) (*testgroup_k8s_io.TestType, error)
	UpdateStatus(*testgroup_k8s_io.TestType) (*testgroup_k8s_io.TestType, error)
	Delete(name string, options *api.DeleteOptions) error
	DeleteCollection(options *api.DeleteOptions, listOptions api.ListOptions) error
	Get(name string) (*testgroup_k8s_io.TestType, error)
	List(opts api.ListOptions) (*testgroup_k8s_io.TestTypeList, error)
	Watch(opts api.ListOptions) (watch.Interface, error)
	TestTypeExpansion
}

// testTypes implements TestTypeInterface
type testTypes struct {
	client *TestgroupClient
	ns     string
}

// newTestTypes returns a TestTypes
func newTestTypes(c *TestgroupClient, namespace string) *testTypes {
	return &testTypes{
		client: c,
		ns:     namespace,
	}
}

// Create takes the representation of a testType and creates it.  Returns the server's representation of the testType, and an error, if there is any.
func (c *testTypes) Create(testType *testgroup_k8s_io.TestType) (result *testgroup_k8s_io.TestType, err error) {
	result = &testgroup_k8s_io.TestType{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("testtypes").
		Body(testType).
		Do().
		Into(result)
	return
}

// Update takes the representation of a testType and updates it. Returns the server's representation of the testType, and an error, if there is any.
func (c *testTypes) Update(testType *testgroup_k8s_io.TestType) (result *testgroup_k8s_io.TestType, err error) {
	result = &testgroup_k8s_io.TestType{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("testtypes").
		Name(testType.Name).
		Body(testType).
		Do().
		Into(result)
	return
}

func (c *testTypes) UpdateStatus(testType *testgroup_k8s_io.TestType) (result *testgroup_k8s_io.TestType, err error) {
	result = &testgroup_k8s_io.TestType{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("testtypes").
		Name(testType.Name).
		SubResource("status").
		Body(testType).
		Do().
		Into(result)
	return
}

// Delete takes name of the testType and deletes it. Returns an error if one occurs.
func (c *testTypes) Delete(name string, options *api.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("testtypes").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *testTypes) DeleteCollection(options *api.DeleteOptions, listOptions api.ListOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("testtypes").
		VersionedParams(&listOptions, api.ParameterCodec).
		Body(options).
		Do().
		Error()
}

// Get takes name of the testType, and returns the corresponding testType object, and an error if there is any.
func (c *testTypes) Get(name string) (result *testgroup_k8s_io.TestType, err error) {
	result = &testgroup_k8s_io.TestType{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("testtypes").
		Name(name).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of TestTypes that match those selectors.
func (c *testTypes) List(opts api.ListOptions) (result *testgroup_k8s_io.TestTypeList, err error) {
	result = &testgroup_k8s_io.TestTypeList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("testtypes").
		VersionedParams(&opts, api.ParameterCodec).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested testTypes.
func (c *testTypes) Watch(opts api.ListOptions) (watch.Interface, error) {
	return c.client.Get().
		Prefix("watch").
		Namespace(c.ns).
		Resource("testtypes").
		VersionedParams(&opts, api.ParameterCodec).
		Watch()
}

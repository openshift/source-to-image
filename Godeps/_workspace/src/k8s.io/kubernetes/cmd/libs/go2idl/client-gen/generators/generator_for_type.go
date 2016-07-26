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

package generators

import (
	"io"
	"path/filepath"
	"strings"

	"k8s.io/kubernetes/cmd/libs/go2idl/generator"
	"k8s.io/kubernetes/cmd/libs/go2idl/namer"
	"k8s.io/kubernetes/cmd/libs/go2idl/types"
)

// genClientForType produces a file for each top-level type.
type genClientForType struct {
	generator.DefaultGen
	outputPackage string
	group         string
	typeToMatch   *types.Type
	imports       namer.ImportTracker
}

var _ generator.Generator = &genClientForType{}

// Filter ignores all but one type because we're making a single file per type.
func (g *genClientForType) Filter(c *generator.Context, t *types.Type) bool { return t == g.typeToMatch }

func (g *genClientForType) Namers(c *generator.Context) namer.NameSystems {
	return namer.NameSystems{
		"raw": namer.NewRawNamer(g.outputPackage, g.imports),
	}
}

func (g *genClientForType) Imports(c *generator.Context) (imports []string) {
	return g.imports.ImportLines()
}

// Ideally, we'd like hasStatus to return true if there is a subresource path
// registered for "status" in the API server, but we do not have that
// information, so hasStatus returns true if the type has a status field.
func hasStatus(t *types.Type) bool {
	for _, m := range t.Members {
		if m.Name == "Status" && strings.Contains(m.Tags, `json:"status`) {
			return true
		}
	}
	return false
}

// GenerateType makes the body of a file implementing the individual typed client for type t.
func (g *genClientForType) GenerateType(c *generator.Context, t *types.Type, w io.Writer) error {
	sw := generator.NewSnippetWriter(w, c, "$", "$")
	pkg := filepath.Base(t.Name.Package)
	namespaced := !(types.ExtractCommentTags("+", t.SecondClosestCommentLines)["nonNamespaced"] == "true")
	m := map[string]interface{}{
		"type":              t,
		"package":           pkg,
		"Package":           namer.IC(pkg),
		"Group":             namer.IC(g.group),
		"watchInterface":    c.Universe.Type(types.Name{Package: "k8s.io/kubernetes/pkg/watch", Name: "Interface"}),
		"apiDeleteOptions":  c.Universe.Type(types.Name{Package: "k8s.io/kubernetes/pkg/api", Name: "DeleteOptions"}),
		"apiListOptions":    c.Universe.Type(types.Name{Package: "k8s.io/kubernetes/pkg/api", Name: "ListOptions"}),
		"apiParameterCodec": c.Universe.Type(types.Name{Package: "k8s.io/kubernetes/pkg/api", Name: "ParameterCodec"}),
		"namespaced":        namespaced,
	}

	sw.Do(getterComment, m)
	if namespaced {
		sw.Do(getterNamesapced, m)
	} else {
		sw.Do(getterNonNamesapced, m)
	}
	noMethods := types.ExtractCommentTags("+", t.SecondClosestCommentLines)["noMethods"] == "true"

	sw.Do(interfaceTemplate1, m)
	if !noMethods {
		sw.Do(interfaceTemplate2, m)
		// Include the UpdateStatus method if the type has a status
		if hasStatus(t) {
			sw.Do(interfaceUpdateStatusTemplate, m)
		}
		sw.Do(interfaceTemplate3, m)
	}
	sw.Do(interfaceTemplate4, m)

	if namespaced {
		sw.Do(structNamespaced, m)
		sw.Do(newStructNamespaced, m)
	} else {
		sw.Do(structNonNamespaced, m)
		sw.Do(newStructNonNamespaced, m)
	}

	if !noMethods {
		sw.Do(createTemplate, m)
		sw.Do(updateTemplate, m)
		// Generate the UpdateStatus method if the type has a status
		if hasStatus(t) {
			sw.Do(updateStatusTemplate, m)
		}
		sw.Do(deleteTemplate, m)
		sw.Do(deleteCollectionTemplate, m)
		sw.Do(getTemplate, m)
		sw.Do(listTemplate, m)
		sw.Do(watchTemplate, m)
	}

	return sw.Error()
}

// group client will implement this interface.
var getterComment = `
// $.type|publicPlural$Getter has a method to return a $.type|public$Interface. 
// A group's client should implement this interface.`

var getterNamesapced = `
type $.type|publicPlural$Getter interface {
	$.type|publicPlural$(namespace string) $.type|public$Interface
}
`

var getterNonNamesapced = `
type $.type|publicPlural$Getter interface {
	$.type|publicPlural$() $.type|public$Interface
}
`

// this type's interface, typed client will implement this interface.
var interfaceTemplate1 = `
// $.type|public$Interface has methods to work with $.type|public$ resources.
type $.type|public$Interface interface {`

var interfaceTemplate2 = `
	Create(*$.type|raw$) (*$.type|raw$, error)
	Update(*$.type|raw$) (*$.type|raw$, error)`

var interfaceUpdateStatusTemplate = `
	UpdateStatus(*$.type|raw$) (*$.type|raw$, error)`

// template for the Interface
var interfaceTemplate3 = `
	Delete(name string, options *$.apiDeleteOptions|raw$) error
	DeleteCollection(options *$.apiDeleteOptions|raw$, listOptions $.apiListOptions|raw$) error
	Get(name string) (*$.type|raw$, error)
	List(opts $.apiListOptions|raw$) (*$.type|raw$List, error)
	Watch(opts $.apiListOptions|raw$) ($.watchInterface|raw$, error)`

var interfaceTemplate4 = `
	$.type|public$Expansion
}
`

// template for the struct that implements the type's interface
var structNamespaced = `
// $.type|privatePlural$ implements $.type|public$Interface
type $.type|privatePlural$ struct {
	client *$.Group$Client
	ns     string
}
`

// template for the struct that implements the type's interface
var structNonNamespaced = `
// $.type|privatePlural$ implements $.type|public$Interface
type $.type|privatePlural$ struct {
	client *$.Group$Client
}
`

var newStructNamespaced = `
// new$.type|publicPlural$ returns a $.type|publicPlural$
func new$.type|publicPlural$(c *$.Group$Client, namespace string) *$.type|privatePlural$ {
	return &$.type|privatePlural${
		client: c,
		ns:     namespace,
	}
}
`

var newStructNonNamespaced = `
// new$.type|publicPlural$ returns a $.type|publicPlural$
func new$.type|publicPlural$(c *$.Group$Client) *$.type|privatePlural$ {
	return &$.type|privatePlural${
		client: c,
	}
}
`

var listTemplate = `
// List takes label and field selectors, and returns the list of $.type|publicPlural$ that match those selectors.
func (c *$.type|privatePlural$) List(opts $.apiListOptions|raw$) (result *$.type|raw$List, err error) {
	result = &$.type|raw$List{}
	err = c.client.Get().
		$if .namespaced$Namespace(c.ns).$end$
		Resource("$.type|allLowercasePlural$").
		VersionedParams(&opts, $.apiParameterCodec|raw$).
		Do().
		Into(result)
	return
}
`
var getTemplate = `
// Get takes name of the $.type|private$, and returns the corresponding $.type|private$ object, and an error if there is any.
func (c *$.type|privatePlural$) Get(name string) (result *$.type|raw$, err error) {
	result = &$.type|raw${}
	err = c.client.Get().
		$if .namespaced$Namespace(c.ns).$end$
		Resource("$.type|allLowercasePlural$").
		Name(name).
		Do().
		Into(result)
	return
}
`

var deleteTemplate = `
// Delete takes name of the $.type|private$ and deletes it. Returns an error if one occurs.
func (c *$.type|privatePlural$) Delete(name string, options *$.apiDeleteOptions|raw$) error {
	return c.client.Delete().
		$if .namespaced$Namespace(c.ns).$end$
		Resource("$.type|allLowercasePlural$").
		Name(name).
		Body(options).
		Do().
		Error()
}
`

var deleteCollectionTemplate = `
// DeleteCollection deletes a collection of objects.
func (c *$.type|privatePlural$) DeleteCollection(options *$.apiDeleteOptions|raw$, listOptions $.apiListOptions|raw$) error {
	return c.client.Delete().
		$if .namespaced$Namespace(c.ns).$end$
		Resource("$.type|allLowercasePlural$").
		VersionedParams(&listOptions, $.apiParameterCodec|raw$).
		Body(options).
		Do().
		Error()
}
`

var createTemplate = `
// Create takes the representation of a $.type|private$ and creates it.  Returns the server's representation of the $.type|private$, and an error, if there is any.
func (c *$.type|privatePlural$) Create($.type|private$ *$.type|raw$) (result *$.type|raw$, err error) {
	result = &$.type|raw${}
	err = c.client.Post().
		$if .namespaced$Namespace(c.ns).$end$
		Resource("$.type|allLowercasePlural$").
		Body($.type|private$).
		Do().
		Into(result)
	return
}
`

var updateTemplate = `
// Update takes the representation of a $.type|private$ and updates it. Returns the server's representation of the $.type|private$, and an error, if there is any.
func (c *$.type|privatePlural$) Update($.type|private$ *$.type|raw$) (result *$.type|raw$, err error) {
	result = &$.type|raw${}
	err = c.client.Put().
		$if .namespaced$Namespace(c.ns).$end$
		Resource("$.type|allLowercasePlural$").
		Name($.type|private$.Name).
		Body($.type|private$).
		Do().
		Into(result)
	return
}
`

var updateStatusTemplate = `
func (c *$.type|privatePlural$) UpdateStatus($.type|private$ *$.type|raw$) (result *$.type|raw$, err error) {
	result = &$.type|raw${}
	err = c.client.Put().
		$if .namespaced$Namespace(c.ns).$end$
		Resource("$.type|allLowercasePlural$").
		Name($.type|private$.Name).
		SubResource("status").
		Body($.type|private$).
		Do().
		Into(result)
	return
}
`

var watchTemplate = `
// Watch returns a $.watchInterface|raw$ that watches the requested $.type|privatePlural$.
func (c *$.type|privatePlural$) Watch(opts $.apiListOptions|raw$) ($.watchInterface|raw$, error) {
	return c.client.Get().
		Prefix("watch").
		$if .namespaced$Namespace(c.ns).$end$
		Resource("$.type|allLowercasePlural$").
		VersionedParams(&opts, $.apiParameterCodec|raw$).
		Watch()
}
`

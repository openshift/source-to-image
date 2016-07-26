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

package fake

import (
	"io"
	"path/filepath"
	"strings"

	"k8s.io/kubernetes/cmd/libs/go2idl/generator"
	"k8s.io/kubernetes/cmd/libs/go2idl/namer"
	"k8s.io/kubernetes/cmd/libs/go2idl/types"
)

// genFakeForType produces a file for each top-level type.
type genFakeForType struct {
	generator.DefaultGen
	outputPackage string
	group         string
	version       string
	typeToMatch   *types.Type
	imports       namer.ImportTracker
}

var _ generator.Generator = &genFakeForType{}

// Filter ignores all but one type because we're making a single file per type.
func (g *genFakeForType) Filter(c *generator.Context, t *types.Type) bool { return t == g.typeToMatch }

func (g *genFakeForType) Namers(c *generator.Context) namer.NameSystems {
	return namer.NameSystems{
		"raw": namer.NewRawNamer(g.outputPackage, g.imports),
	}
}

func (g *genFakeForType) Imports(c *generator.Context) (imports []string) {
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

// hasObjectMeta returns true if the type has a ObjectMeta field.
func hasObjectMeta(t *types.Type) bool {
	for _, m := range t.Members {
		if m.Embedded == true && m.Name == "ObjectMeta" {
			return true
		}
	}
	return false
}

// GenerateType makes the body of a file implementing the individual typed client for type t.
func (g *genFakeForType) GenerateType(c *generator.Context, t *types.Type, w io.Writer) error {
	sw := generator.NewSnippetWriter(w, c, "$", "$")
	pkg := filepath.Base(t.Name.Package)
	const pkgTestingCore = "k8s.io/kubernetes/pkg/client/testing/core"
	namespaced := !(types.ExtractCommentTags("+", t.SecondClosestCommentLines)["nonNamespaced"] == "true")
	canonicalGroup := g.group
	if canonicalGroup == "core" {
		canonicalGroup = ""
	}
	canonicalVersion := g.version
	if canonicalVersion == "unversioned" {
		canonicalVersion = ""
	}
	m := map[string]interface{}{
		"type":                 t,
		"package":              pkg,
		"Package":              namer.IC(pkg),
		"namespaced":           namespaced,
		"Group":                namer.IC(g.group),
		"group":                canonicalGroup,
		"version":              canonicalVersion,
		"watchInterface":       c.Universe.Type(types.Name{Package: "k8s.io/kubernetes/pkg/watch", Name: "Interface"}),
		"apiDeleteOptions":     c.Universe.Type(types.Name{Package: "k8s.io/kubernetes/pkg/api", Name: "DeleteOptions"}),
		"apiListOptions":       c.Universe.Type(types.Name{Package: "k8s.io/kubernetes/pkg/api", Name: "ListOptions"}),
		"GroupVersionResource": c.Universe.Type(types.Name{Package: "k8s.io/kubernetes/pkg/api/unversioned", Name: "GroupVersionResource"}),
		"Everything":           c.Universe.Function(types.Name{Package: "k8s.io/kubernetes/pkg/labels", Name: "Everything"}),

		"NewRootListAction":              c.Universe.Function(types.Name{Package: pkgTestingCore, Name: "NewRootListAction"}),
		"NewListAction":                  c.Universe.Function(types.Name{Package: pkgTestingCore, Name: "NewListAction"}),
		"NewRootGetAction":               c.Universe.Function(types.Name{Package: pkgTestingCore, Name: "NewRootGetAction"}),
		"NewGetAction":                   c.Universe.Function(types.Name{Package: pkgTestingCore, Name: "NewGetAction"}),
		"NewRootDeleteAction":            c.Universe.Function(types.Name{Package: pkgTestingCore, Name: "NewRootDeleteAction"}),
		"NewDeleteAction":                c.Universe.Function(types.Name{Package: pkgTestingCore, Name: "NewDeleteAction"}),
		"NewRootDeleteCollectionAction":  c.Universe.Function(types.Name{Package: pkgTestingCore, Name: "NewRootDeleteCollectionAction"}),
		"NewDeleteCollectionAction":      c.Universe.Function(types.Name{Package: pkgTestingCore, Name: "NewDeleteCollectionAction"}),
		"NewRootUpdateAction":            c.Universe.Function(types.Name{Package: pkgTestingCore, Name: "NewRootUpdateAction"}),
		"NewUpdateAction":                c.Universe.Function(types.Name{Package: pkgTestingCore, Name: "NewUpdateAction"}),
		"NewRootCreateAction":            c.Universe.Function(types.Name{Package: pkgTestingCore, Name: "NewRootCreateAction"}),
		"NewCreateAction":                c.Universe.Function(types.Name{Package: pkgTestingCore, Name: "NewCreateAction"}),
		"NewRootWatchAction":             c.Universe.Function(types.Name{Package: pkgTestingCore, Name: "NewRootWatchAction"}),
		"NewWatchAction":                 c.Universe.Function(types.Name{Package: pkgTestingCore, Name: "NewWatchAction"}),
		"NewUpdateSubresourceAction":     c.Universe.Function(types.Name{Package: pkgTestingCore, Name: "NewUpdateSubresourceAction"}),
		"NewRootUpdateSubresourceAction": c.Universe.Function(types.Name{Package: pkgTestingCore, Name: "NewRootUpdateSubresourceAction"}),
	}

	noMethods := types.ExtractCommentTags("+", t.SecondClosestCommentLines)["noMethods"] == "true"

	if namespaced {
		sw.Do(structNamespaced, m)
	} else {
		sw.Do(structNonNamespaced, m)
	}

	if !noMethods {
		sw.Do(resource, m)
		sw.Do(createTemplate, m)
		sw.Do(updateTemplate, m)
		// Generate the UpdateStatus method if the type has a status
		if hasStatus(t) {
			sw.Do(updateStatusTemplate, m)
		}
		sw.Do(deleteTemplate, m)
		sw.Do(deleteCollectionTemplate, m)
		sw.Do(getTemplate, m)
		if hasObjectMeta(t) {
			sw.Do(listUsingOptionsTemplate, m)
		} else {
			sw.Do(listTemplate, m)
		}
		sw.Do(watchTemplate, m)

	}

	return sw.Error()
}

// template for the struct that implements the type's interface
var structNamespaced = `
// Fake$.type|publicPlural$ implements $.type|public$Interface
type Fake$.type|publicPlural$ struct {
	Fake *Fake$.Group$
	ns     string
}
`

// template for the struct that implements the type's interface
var structNonNamespaced = `
// Fake$.type|publicPlural$ implements $.type|public$Interface
type Fake$.type|publicPlural$ struct {
	Fake *Fake$.Group$
}
`

var resource = `
var $.type|allLowercasePlural$Resource = $.GroupVersionResource|raw${Group: "$.group$", Version: "$.version$", Resource: "$.type|allLowercasePlural$"}
`

var listTemplate = `
func (c *Fake$.type|publicPlural$) List(opts $.apiListOptions|raw$) (result *$.type|raw$List, err error) {
	obj, err := c.Fake.
		$if .namespaced$Invokes($.NewListAction|raw$($.type|allLowercasePlural$Resource, c.ns, opts), &$.type|raw$List{})
		$else$Invokes($.NewRootListAction|raw$($.type|allLowercasePlural$Resource, opts), &$.type|raw$List{})$end$
	if obj == nil {
		return nil, err
	}
	return obj.(*$.type|raw$List), err
}
`

var listUsingOptionsTemplate = `
func (c *Fake$.type|publicPlural$) List(opts $.apiListOptions|raw$) (result *$.type|raw$List, err error) {
	obj, err := c.Fake.
		$if .namespaced$Invokes($.NewListAction|raw$($.type|allLowercasePlural$Resource, c.ns, opts), &$.type|raw$List{})
		$else$Invokes($.NewRootListAction|raw$($.type|allLowercasePlural$Resource, opts), &$.type|raw$List{})$end$
	if obj == nil {
		return nil, err
	}

	label := opts.LabelSelector
	if label == nil {
		label = $.Everything|raw$()
	}
	list := &$.type|raw$List{}
	for _, item := range obj.(*$.type|raw$List).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}
`

var getTemplate = `
func (c *Fake$.type|publicPlural$) Get(name string) (result *$.type|raw$, err error) {
	obj, err := c.Fake.
		$if .namespaced$Invokes($.NewGetAction|raw$($.type|allLowercasePlural$Resource, c.ns, name), &$.type|raw${})
		$else$Invokes($.NewRootGetAction|raw$($.type|allLowercasePlural$Resource, name), &$.type|raw${})$end$
	if obj == nil {
		return nil, err
	}
	return obj.(*$.type|raw$), err
}
`

var deleteTemplate = `
func (c *Fake$.type|publicPlural$) Delete(name string, options *$.apiDeleteOptions|raw$) error {
	_, err := c.Fake.
		$if .namespaced$Invokes($.NewDeleteAction|raw$($.type|allLowercasePlural$Resource, c.ns, name), &$.type|raw${})
		$else$Invokes($.NewRootDeleteAction|raw$($.type|allLowercasePlural$Resource, name), &$.type|raw${})$end$
	return err
}
`

var deleteCollectionTemplate = `
func (c *Fake$.type|publicPlural$) DeleteCollection(options *$.apiDeleteOptions|raw$, listOptions $.apiListOptions|raw$) error {
	$if .namespaced$action := $.NewDeleteCollectionAction|raw$($.type|allLowercasePlural$Resource, c.ns, listOptions)
	$else$action := $.NewRootDeleteCollectionAction|raw$($.type|allLowercasePlural$Resource, listOptions)
	$end$
	_, err := c.Fake.Invokes(action, &$.type|raw$List{})
	return err
}
`

var createTemplate = `
func (c *Fake$.type|publicPlural$) Create($.type|private$ *$.type|raw$) (result *$.type|raw$, err error) {
	obj, err := c.Fake.
		$if .namespaced$Invokes($.NewCreateAction|raw$($.type|allLowercasePlural$Resource, c.ns, $.type|private$), &$.type|raw${})
		$else$Invokes($.NewRootCreateAction|raw$($.type|allLowercasePlural$Resource, $.type|private$), &$.type|raw${})$end$
	if obj == nil {
		return nil, err
	}
	return obj.(*$.type|raw$), err
}
`

var updateTemplate = `
func (c *Fake$.type|publicPlural$) Update($.type|private$ *$.type|raw$) (result *$.type|raw$, err error) {
	obj, err := c.Fake.
		$if .namespaced$Invokes($.NewUpdateAction|raw$($.type|allLowercasePlural$Resource, c.ns, $.type|private$), &$.type|raw${})
		$else$Invokes($.NewRootUpdateAction|raw$($.type|allLowercasePlural$Resource, $.type|private$), &$.type|raw${})$end$
	if obj == nil {
		return nil, err
	}
	return obj.(*$.type|raw$), err
}
`

var updateStatusTemplate = `
func (c *Fake$.type|publicPlural$) UpdateStatus($.type|private$ *$.type|raw$) (*$.type|raw$, error) {
	obj, err := c.Fake.
		$if .namespaced$Invokes($.NewUpdateSubresourceAction|raw$($.type|allLowercasePlural$Resource, "status", c.ns, $.type|private$), &$.type|raw${})
		$else$Invokes($.NewRootUpdateSubresourceAction|raw$($.type|allLowercasePlural$Resource, "status", $.type|private$), &$.type|raw${})$end$
	if obj == nil {
		return nil, err
	}
	return obj.(*$.type|raw$), err
}
`

var watchTemplate = `
// Watch returns a $.watchInterface|raw$ that watches the requested $.type|privatePlural$.
func (c *Fake$.type|publicPlural$) Watch(opts $.apiListOptions|raw$) ($.watchInterface|raw$, error) {
	return c.Fake.
		$if .namespaced$InvokesWatch($.NewWatchAction|raw$($.type|allLowercasePlural$Resource, c.ns, opts))
		$else$InvokesWatch($.NewRootWatchAction|raw$($.type|allLowercasePlural$Resource, opts))$end$
}
`

/*
Copyright 2023 Go Imports Organizer Contributors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package sorter

import (
	"go/ast"

	v1alpha1 "github.com/go-imports-organizer/goio/pkg/api/v1alpha1"
)

// SortImportsByPathValue sorts a slice of ImportSpecs using their Path.Value
type SortImportsByPathValue []ast.ImportSpec

func (a SortImportsByPathValue) Len() int           { return len(a) }
func (a SortImportsByPathValue) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a SortImportsByPathValue) Less(i, j int) bool { return a[i].Path.Value < a[j].Path.Value }

// SortGroupsByMatchOrder sorts a slice of Group objects using their MatchOrder
type SortGroupsByMatchOrder []v1alpha1.Group

func (a SortGroupsByMatchOrder) Len() int           { return len(a) }
func (a SortGroupsByMatchOrder) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a SortGroupsByMatchOrder) Less(i, j int) bool { return a[i].MatchOrder < a[j].MatchOrder }

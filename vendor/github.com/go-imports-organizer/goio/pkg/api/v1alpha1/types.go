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
package v1alpha1

import (
	"regexp"
	"strings"
)

// RegExpMatcher
type RegExpMatcher struct {
	Bucket string         `yaml:"bucket"`
	RegExp *regexp.Regexp `yaml:"regexp"`
}

const (
	// ExcludeMatchTypeName tells an Exclude to match against the file or folder name
	ExcludeMatchTypeName string = "name"
	// ExcludeMatchTypeRelativePath tells an Exclude to match against the file or folder path
	ExcludeMatchTypeRelativePath string = "path"
)

// Exclude defines a file or folder that should be excluded from being organized
type Exclude struct {
	// MatchType defines whether the file name or file path should be matched against
	MatchType string `yaml:"matchtype"`
	// RegExp is the Regular Expression that is used to match against
	RegExp string `yaml:"regexp"`
}

// Group defines a block of imports
type Group struct {
	// MatchOrder is the order is which the Regular Expression will be matched
	// against an import to determine its group
	MatchOrder int `yaml:"matchorder"`
	// Description is a friendly name for the group
	Description string `yaml:"description"`
	// RegExp is the Regular Expression that is used to match against the imports Path.Value
	RegExp []string `yaml:"regexp"`
}

// Config is the configuration for the Go Imports Organizer
type Config struct {
	// Excludes is a slice of Exclude objects
	Excludes []Exclude `yaml:"excludes"`
	// Groups is a slice of Group objects
	Groups []Group `yaml:"groups"`
}

// PathListFlags is a type that can store Path objects that are supplied via the -p flag
type PathListFlags []string

func (i *PathListFlags) String() string {
	return strings.Join(*i, ",")
}

func (i *PathListFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

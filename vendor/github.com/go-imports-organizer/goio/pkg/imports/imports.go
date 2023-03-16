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
package imports

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/scanner"
	"go/token"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	v1alpha1 "github.com/go-imports-organizer/goio/pkg/api/v1alpha1"
	"github.com/go-imports-organizer/goio/pkg/sorter"
)

// AddSpaces adds empty lines (spaces) between the groups of imports
// borrowed from https://github.com/golang/tools/blob/71482053b885ea3938876d1306ad8a1e4037f367/internal/imports/imports.go#L380
func AddSpaces(r io.Reader, breaks []string) ([]byte, error) {
	var out bytes.Buffer
	in := bufio.NewReader(r)
	inImports := false
	done := false
	for {
		s, err := in.ReadString('\n')
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}

		if !inImports && !done && strings.HasPrefix(s, "import") {
			inImports = true
		}
		if inImports && (strings.HasPrefix(s, "var") ||
			strings.HasPrefix(s, "func") ||
			strings.HasPrefix(s, "const") ||
			strings.HasPrefix(s, "type")) {
			done = true
			inImports = false
		}
		if inImports && len(breaks) > 0 {
			if m := regexp.MustCompile(`^\s+(?:[\w\.]+\s+)?"(.+)"`).FindStringSubmatch(s); m != nil {
				if m[1] == breaks[0] {
					out.WriteByte('\n')
					breaks = breaks[1:]
				}
			}
		}
		fmt.Fprint(&out, s)
	}
	return out.Bytes(), nil
}

// PopulateGroups assembles the data structure that is used to hold the groups
// of ImportSpecs as they are organized
func PopulateGroups(importGroups map[string][]ast.ImportSpec, regExpMatchers []v1alpha1.RegExpMatcher, imports []*ast.ImportSpec) error {
	for _, i := range imports {
		if len(i.Path.Value) == 0 {
			continue
		}
		found := false
		unquotedPath, err := strconv.Unquote(i.Path.Value)
		if err != nil {
			return fmt.Errorf("unable to unquote %s", i.Path.Value)
		}
		for _, r := range regExpMatchers {
			if r.RegExp.MatchString(unquotedPath) {
				if _, ok := importGroups[r.Bucket]; !ok {
					importGroups[r.Bucket] = []ast.ImportSpec{}
				}
				importGroups[r.Bucket] = append(importGroups[r.Bucket], *i)
				found = true
				break
			}
		}
		if !found {
			importGroups["other"] = append(importGroups["other"], *i)
		}
	}
	return nil
}

// InsertGroup places the groups of ImportSpecs into their correct order in the
// File according to the display order
func InsertGroups(f *ast.File, importGroups map[string][]ast.ImportSpec, displayOrder []string) ([]string, error) {
	var breaks []string
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if ok && gen.Tok == token.IMPORT {
			gen.Specs = []ast.Spec{}
			for _, group := range displayOrder {
				sort.Sort(sorter.SortImportsByPathValue(importGroups[group]))
				for n := range importGroups[group] {
					importGroups[group][n].EndPos = 0
					importGroups[group][n].Path.ValuePos = 0
					if importGroups[group][n].Name != nil {
						importGroups[group][n].Name.NamePos = 0
					}
					gen.Specs = append(gen.Specs, &importGroups[group][n])
					if n == 0 && group != displayOrder[0] {
						newstr, err := strconv.Unquote(importGroups[group][n].Path.Value)
						if err != nil {
							return nil, err
						}
						breaks = append(breaks, newstr)
					}
				}
			}
		}
	}
	return breaks, nil
}

// Format processes files as they are added to the queue and organizes the imports
func Format(files *chan string, wg *sync.WaitGroup, groupRegExpMatchers []v1alpha1.RegExpMatcher, displayOrder []string, listOnly *bool) {
	defer wg.Done()
	for path := range *files {
		if len(path) == 0 {
			continue
		}

		info, err := os.Stat(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to stat %q", err.Error())
			continue
		}
		oldModTime := info.ModTime()

		fs := token.NewFileSet()
		f, err := parser.ParseFile(fs, path, nil, parser.ParseComments)
		if err != nil {
			var scannerErrorList scanner.ErrorList
			if errors.As(err, &scannerErrorList) {
				for _, err := range scannerErrorList {
					fmt.Fprintf(os.Stderr, "%s", err)
				}
			} else {
				fmt.Fprintf(os.Stderr, "%s", err.Error())
			}
		}

		var importGroups = make(map[string][]ast.ImportSpec)
		if err := PopulateGroups(importGroups, groupRegExpMatchers, f.Imports); err != nil {
			fmt.Fprintf(os.Stderr, "unable to populate import groups for %q: %s", path, err)
			continue
		}

		breaks, err := InsertGroups(f, importGroups, displayOrder)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to update groups at %q: %s", path, err.Error())
			continue
		}

		printerMode := printer.TabIndent

		printConfig := &printer.Config{Mode: printerMode, Tabwidth: 4}

		var buf bytes.Buffer
		if err = printConfig.Fprint(&buf, fs, f); err != nil {
			fmt.Fprintf(os.Stderr, "unable to load bytes into buffer %q, %s", path, err.Error())
			continue
		}
		out, err := AddSpaces(bytes.NewReader(buf.Bytes()), breaks)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to add spaces to %q, %s", path, err.Error())
			continue
		}
		out, err = format.Source(out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to format source %q, %s", path, err.Error())
		}

		if *listOnly {
			oldFile, err := os.ReadFile(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "unable to read file %q: %s", path, err.Error())
				continue
			}
			if !bytes.Equal(oldFile, out) {
				fmt.Fprintf(os.Stdout, "%s is not organized \n", path)
			}
		}

		info, err = os.Stat(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to stat %q: %s", path, err.Error())
			continue
		}
		if !info.ModTime().Equal(oldModTime) {
			fmt.Fprintf(os.Stderr, "%s was modified while formatting, cowardly refusing to overwrite", path)
			continue
		}
		if err = os.WriteFile(path, out, info.Mode()); err != nil {
			fmt.Fprintf(os.Stderr, "unable to write to path %q, %s", path, err.Error())
		}
	}
}

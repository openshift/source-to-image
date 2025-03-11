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
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"

	v1alpha1 "github.com/go-imports-organizer/goio/pkg/api/v1alpha1"
	"github.com/go-imports-organizer/goio/pkg/config"
	"github.com/go-imports-organizer/goio/pkg/excludes"
	"github.com/go-imports-organizer/goio/pkg/groups"
	"github.com/go-imports-organizer/goio/pkg/imports"
	"github.com/go-imports-organizer/goio/pkg/module"
	"github.com/go-imports-organizer/goio/pkg/version"
)

var (
	wg          sync.WaitGroup
	files       = make(chan string)
	resultsChan = make(chan string)
	hasResults  = false
	pathList    v1alpha1.PathListFlags
)

func main() {
	listOnly := flag.Bool("l", false, "only list files that need to be organized (no changes made)")
	flag.Var(&pathList, "p", "specify individual paths to organize, use multiple times for multiple paths. defaults to entire module directory")
	versionOnly := flag.Bool("v", false, "print version and exit")
	flag.Parse()

	// set CPUPROFILE=<filename> to create a <filename>.pprof cpu profile file
	if len(os.Getenv("CPUPROFILE")) != 0 {
		fmt.Fprintf(os.Stdout, "Logging CPU profiling information to %s\n", os.Getenv(("CPUPROFILE")))
		f, err := os.Create(os.Getenv("CPUPROFILE"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s", err.Error())
			os.Exit(1)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	if *versionOnly {
		fmt.Fprintf(os.Stdout, "%s\n", version.Get())
		os.Exit(0)
	}

	currentDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to get current working directory: %s\n", err.Error())
		os.Exit(1)
	}

	// Find the Go module name and path
	goModuleName, goModulePath, err := module.FindGoModuleNameAndPath(currentDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error occurred finding module path: %s\n", err.Error())
		os.Exit(1)
	}

	// Find a goio.yaml file in the current directory or any parent directory
	path, found, err := findFile(currentDir, "goio.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error occurred finding configuration file goio.yaml: %v\n", err)
		os.Exit(1)
	}
	if !found {
		fmt.Fprint(os.Stderr, "error occurred finding configuration file goio.yaml\n")
		os.Exit(1)
	}

	// Load the configuration from the goio.yaml file
	conf, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error occurred loading configuration file: %s\n", err.Error())
		os.Exit(1)
	}

	// Build the Regular Expressions for excluding files/folders
	excludeByNameRegExp, excludeByPathRegExp := excludes.Build(conf.Excludes)

	// Build the Regular Expressions and DisplayOrder for the group definitions
	groupRegExpMatchers, displayOrder := groups.Build(conf.Groups, goModuleName)

	// Read results from the resultsChan and write them to stdout
	go func() {
		for {
			r := <-resultsChan
			if len(r) != 0 {
				fmt.Fprintf(os.Stdout, "%s\n", r)
			}
		}
	}()

	// Add one (1) to the WaitGroup so that we can know when the Formatting in completed
	wg.Add(1)

	// Start up the Format worker so that it is ready when we start queuing up files
	go imports.Format(&files, &resultsChan, &hasResults, &wg, groupRegExpMatchers, displayOrder, listOnly)

	// Set the basePath for use later
	basePath := goModulePath + "/"

	// Change our working directory to the goModulePath
	err = os.Chdir(goModulePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to change directory to %q: %s\n", goModulePath, err.Error())
		os.Exit(1)
	}

	// Pre-optization so that we can skip the Name or Path matches if they are empty
	excludeByNameRegExpLenOk := len(excludeByNameRegExp.String()) != 0
	excludeByPathRegExpLenOk := len(excludeByPathRegExp.String()) != 0

	// If no paths are supplied via the -p flag use the current directory
	if len(pathList) == 0 {
		pathList = append(pathList, goModulePath)
	}

	for _, path := range pathList {
		f, err := os.Stat(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to stat %q: %s\n", path, err.Error())
			continue
		}

		// If the path is a Go file
		if strings.HasSuffix(path, ".go") {
			// If the files name or path matches an exclude Regular Expression, skip it
			if (excludeByNameRegExpLenOk && excludeByNameRegExp.MatchString(f.Name())) || (excludeByPathRegExpLenOk && excludeByPathRegExp.MatchString(path)) {
				continue
			}
			// If the file is not excluded by name or path, queue it for organizing
			files <- path

		} else if f.IsDir() {
			// If the path is a directory
			if err = filepath.Walk(path, func(path string, f os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				name := f.Name()
				isDir := f.IsDir()
				isGoFile := strings.HasSuffix(name, ".go")
				relativePath := strings.Replace(path, basePath, "", 1)
				// If the object is not a directory and not a Go file, skip it
				if isDir || isGoFile {
					// If the objects name or path matches an exclude Regular Expression, skip it
					if (excludeByNameRegExpLenOk && excludeByNameRegExp.MatchString(name)) || (excludeByPathRegExpLenOk && excludeByPathRegExp.MatchString(relativePath)) {
						// If the object is a Directory, skip the entire thing
						if isDir {
							return filepath.SkipDir
						}
						return nil
					}

					// If the object is a Go file and is not excluded, queue it for organizing
					if isGoFile {
						files <- relativePath
					}
				}
				return nil
			}); err != nil {
				fmt.Fprintf(os.Stderr, "unable to complete walking file tree: %s\n", err.Error())
			}
		}

	}

	// Close the files channel since we are done queuing up files to format
	close(files)

	// Wait for all files to be processed
	wg.Wait()

	// Close the resultsChan as all formatting should be completed
	close(resultsChan)

	// set MEMPROFILE=<filename> to create a <filename>.pprof memory profile file
	if len(os.Getenv("MEMPROFILE")) != 0 {
		fmt.Fprintf(os.Stdout, "Logging MEMORY profiling information to %s\n", os.Getenv(("MEMPROFILE")))
		f, err := os.Create(os.Getenv("MEMPROFILE"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s", err.Error())
			os.Exit(1)
		}
		defer f.Close()
		runtime.GC()
		if err := pprof.WriteHeapProfile(f); err != nil {
			fmt.Fprintf(os.Stderr, "could not write memory profile: %s", err.Error())
			os.Exit(1)
		}
	}
	if *listOnly && hasResults {
		os.Exit(1)
	}
}

func findFile(path, fileName string) (string, bool, error) {
	for {
		_, err := os.Stat(filepath.Join(path, fileName))
		if err == nil {
			return filepath.Join(path, fileName), true, nil
		}
		if !os.IsNotExist(err) {
			return "", false, err
		}
		if path == "/" {
			return "", false, nil
		}
		path = filepath.Dir(path)
	}
}

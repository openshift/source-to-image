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
package module

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

// FindGoModuleNameAndPath finds the current Go modules name (via the go.mod file)
// and path (location of the go.mod file on the filesystem)
func FindGoModuleNameAndPath(path string) (string, string, error) {
	for path != "." {
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				return "", "", fmt.Errorf("%s does not exist: %s", path, err.Error())
			}
		}
		if _, err := os.Stat(fmt.Sprintf("%s/go.mod", path)); err != nil {
			path = filepath.Dir(path)
			continue
		}
		break
	}

	f, err := os.ReadFile(fmt.Sprintf("%s/go.mod", path))
	if err != nil {
		return "", "", fmt.Errorf("unable to open go.mod file for reading: %v", err)
	}
	module := modfile.ModulePath(f)
	if len(module) == 0 {
		return "", "", fmt.Errorf("unable to determine module")
	}
	return module, path, nil
}

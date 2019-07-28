package util

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/util/fs"
)

// FixInjectionsWithRelativePath fixes the injections that does not specify the
// destination directory or the directory is relative to use the provided
// working directory.
func FixInjectionsWithRelativePath(workdir string, injections api.VolumeList) api.VolumeList {
	if len(injections) == 0 {
		return injections
	}
	newList := api.VolumeList{}
	for _, injection := range injections {
		changed := false
		if filepath.Clean(filepath.FromSlash(injection.Destination)) == "." {
			injection.Destination = filepath.ToSlash(workdir)
			changed = true
		}
		if filepath.ToSlash(injection.Destination)[0] != '/' {
			injection.Destination = filepath.ToSlash(filepath.Join(workdir, injection.Destination))
			changed = true
		}
		if changed {
			log.V(5).Infof("Using %q as a destination for injecting %q", injection.Destination, injection.Source)
		}
		newList = append(newList, injection)
	}
	return newList
}

// ListFilesToTruncate returns a flat list of all files that are injected into a
// container which need to be truncated. All files from nested directories are returned in the list.
func ListFilesToTruncate(fs fs.FileSystem, injections api.VolumeList) ([]string, error) {
	result := []string{}
	for _, s := range injections {
		if s.Keep {
			continue
		}
		files, err := ListFiles(fs, s)
		if err != nil {
			return nil, err
		}
		result = append(result, files...)
	}
	return result, nil
}

// ListFiles returns a flat list of all files injected into a container for the given `VolumeSpec`.
func ListFiles(fs fs.FileSystem, spec api.VolumeSpec) ([]string, error) {
	result := []string{}
	if _, err := os.Stat(spec.Source); err != nil {
		return nil, err
	}
	err := fs.Walk(spec.Source, func(path string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Detected files will be truncated. k8s' AtomicWriter creates
		// directories and symlinks to directories in order to inject files.
		// An attempt to truncate either a dir or symlink to a dir will fail.
		// Thus, we need to dereference symlinks to see if they might point
		// to a directory.
		// Do not try to simplify this logic to simply return nil if a symlink
		// is detected. During the tar transfer to an assemble image, symlinked
		// files are turned concrete (i.e. they will be turned into regular files
		// containing the content of their target). These newly concrete files
		// need to be truncated as well.

		if f.Mode()&os.ModeSymlink != 0 {
			linkDest, err := filepath.EvalSymlinks(path)
			if err != nil {
				return fmt.Errorf("unable to evaluate symlink [%v]: %v", path, err)
			}
			// Evaluate the destination of the link.
			f, err = os.Lstat(linkDest)
			if err != nil {
				// This is not a fatal error. If AtomicWrite tried multiple times, a symlink might not point
				// to a valid destination.
				log.Warningf("Unable to lstat symlink destination [%s]->[%s]. err: %v. Partial atomic write?", path, linkDest, err)
				return nil
			}
		}

		if f.IsDir() {
			return nil
		}

		newPath := filepath.ToSlash(filepath.Join(spec.Destination, strings.TrimPrefix(path, spec.Source)))
		result = append(result, newPath)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// CreateTruncateFilesScript creates a shell script that contains truncation
// of all files we injected into the container. The path to the script is returned.
// When the scriptName is provided, it is also truncated together with all
// secrets.
func CreateTruncateFilesScript(files []string, scriptName string) (string, error) {
	rmScript := "set -e\n"
	for _, s := range files {
		rmScript += fmt.Sprintf("truncate -s0 %q\n", s)
	}

	f, err := ioutil.TempFile("", "s2i-injection-remove")
	if err != nil {
		return "", err
	}
	if len(scriptName) > 0 {
		rmScript += fmt.Sprintf("truncate -s0 %q\n", scriptName)
	}
	rmScript += "set +e\n"
	err = ioutil.WriteFile(f.Name(), []byte(rmScript), 0700)
	return f.Name(), err
}

// CreateInjectionResultFile creates a result file with the message from the provided injection
// error. The path to the result file is returned. If the provided error is nil, an empty file is
// created.
func CreateInjectionResultFile(injectErr error) (string, error) {
	f, err := ioutil.TempFile("", "s2i-injection-result")
	if err != nil {
		return "", err
	}
	if injectErr != nil {
		err = ioutil.WriteFile(f.Name(), []byte(injectErr.Error()), 0700)
	}
	return f.Name(), err
}

// HandleInjectionError handles the error caused by injection and provide
// reasonable suggestion to users.
func HandleInjectionError(p api.VolumeSpec, err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "no such file or directory") {
		log.Errorf("The destination directory for %q injection must exist in container (%q)", p.Source, p.Destination)
		return err
	}
	log.Errorf("Error occurred during injecting %q to %q: %v", p.Source, p.Destination, err)
	return err
}

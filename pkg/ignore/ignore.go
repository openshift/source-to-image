package ignore

import (
	"bufio"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/api/constants"
	utillog "github.com/openshift/source-to-image/pkg/util/log"
)

var log = utillog.StderrLog

// DockerIgnorer ignores files based on the contents of the .s2iignore file
type DockerIgnorer struct{}

// Ignore removes files from the workspace based on the contents of the
// .s2iignore file
func (b *DockerIgnorer) Ignore(config *api.Config) error {
	/*
		 so, to duplicate the .dockerignore capabilities (https://docs.docker.com/reference/builder/#dockerignore-file)
		 we have a flow that follows:
		0) First note, .dockerignore rules are NOT recursive (unlike .gitignore) .. you have to list subdir explicitly
		1) Read in the exclusion patterns
		2) Skip over comments (noted by #)
		3) note overrides (via exclamation sign i.e. !) and reinstate files (don't remove) as needed
		4) leverage Glob matching to build list, as .dockerignore is documented as following filepath.Match / filepath.Glob
		5) del files
		 1 to 4 is in getListOfFilesToIgnore
	*/
	filesToDel, lerr := b.GetListOfFilesToIgnore(config.WorkingSourceDir)
	if lerr != nil {
		return lerr
	}

	if filesToDel == nil {
		return nil
	}

	// delete compiled list of files
	for _, fileToDel := range filesToDel {
		log.V(5).Infof("attempting to remove file %s \n", fileToDel)
		rerr := os.RemoveAll(fileToDel)
		if rerr != nil {
			log.Errorf("error removing file %s because of %v \n", fileToDel, rerr)
			return rerr
		}
	}

	return nil
}

// GetListOfFilesToIgnore returns list of files from the workspace based on the contents of the
// .s2iignore file
func (b *DockerIgnorer) GetListOfFilesToIgnore(workingDir string) (map[string]string, error) {
	path := filepath.Join(workingDir, constants.IgnoreFile)
	m, err := NewMatcher(path)
	if err != nil {
		return nil, err
	}

	filesToDel := make(map[string]string)
	err = filepath.Walk(workingDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return nil // ignore error
		}

		rp, err := filepath.Rel(workingDir, path)
		if err != nil {
			return err
		}

		if rp == "." {
			return nil
		}

		if m.Match(rp) {
			filesToDel[path] = path
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return filesToDel, nil
}

type fileSpec struct {
	glob    string
	inverse bool
}

type Matcher struct {
	specs []fileSpec
}

func NewMatcher(s2iIgnorePath string) (Matcher, error) {
	file, err := os.Open(s2iIgnorePath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Errorf("Ignore processing, problem opening %s because of %v\n", s2iIgnorePath, err)
			return Matcher{}, err
		}
		log.V(4).Info(".s2iignore file does not exist")
		return Matcher{}, nil
	}
	defer file.Close()

	var specs []fileSpec
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		filespec := strings.Trim(scanner.Text(), " ")

		if len(filespec) == 0 {
			continue
		}

		if strings.HasPrefix(filespec, "#") {
			continue
		}

		log.V(4).Infof(".s2iignore lists a file spec of %s \n", filespec)

		if strings.HasPrefix(filespec, "!") {
			filespec = strings.Replace(filespec, "!", "", 1)
			specs = append(specs, fileSpec{
				glob:    filespec,
				inverse: true,
			})
			continue
		}

		specs = append(specs, fileSpec{glob: filespec})
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		log.Errorf("Problem processing .s2iignore %v \n", err)
		return Matcher{}, err
	}
	return Matcher{specs: specs}, nil
}

func (m Matcher) Match(path string) bool {
	var matches bool
	for _, spec := range m.specs {
		if ok, _ := filepath.Match(spec.glob, path); ok {
			matches = !spec.inverse
		}
	}
	return matches
}

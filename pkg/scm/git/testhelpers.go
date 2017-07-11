package git

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/openshift/source-to-image/pkg/util/cmd"
	"github.com/openshift/source-to-image/pkg/util/cygpath"
)

// CreateLocalGitDirectory creates a git directory with a commit
func CreateLocalGitDirectory(t *testing.T) string {
	cr := cmd.NewCommandRunner()
	dir := CreateEmptyLocalGitDirectory(t)
	f, err := os.Create(filepath.Join(dir, "testfile"))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	err = cr.RunWithOptions(cmd.CommandOpts{Dir: dir}, "git", "add", ".")
	if err != nil {
		t.Fatal(err)
	}
	err = cr.RunWithOptions(cmd.CommandOpts{Dir: dir, EnvAppend: []string{"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test"}}, "git", "commit", "-m", "testcommit")
	if err != nil {
		t.Fatal(err)
	}

	return dir
}

// CreateEmptyLocalGitDirectory creates a git directory with no checkin yet
func CreateEmptyLocalGitDirectory(t *testing.T) string {
	cr := cmd.NewCommandRunner()

	dir, err := ioutil.TempDir(os.TempDir(), "gitdir-s2i-test")
	if err != nil {
		t.Fatal(err)
	}
	err = cr.RunWithOptions(cmd.CommandOpts{Dir: dir}, "git", "init")
	if err != nil {
		t.Fatal(err)
	}

	return dir
}

// CreateLocalGitDirectoryWithSubmodule creates a git directory with a submodule
func CreateLocalGitDirectoryWithSubmodule(t *testing.T) string {
	cr := cmd.NewCommandRunner()

	submodule := CreateLocalGitDirectory(t)
	defer os.RemoveAll(submodule)

	if cygpath.UsingCygwinGit {
		var err error
		submodule, err = cygpath.ToSlashCygwin(submodule)
		if err != nil {
			t.Fatal(err)
		}
	}

	dir := CreateEmptyLocalGitDirectory(t)
	err := cr.RunWithOptions(cmd.CommandOpts{Dir: dir}, "git", "submodule", "add", submodule, "submodule")
	if err != nil {
		t.Fatal(err)
	}

	return dir
}

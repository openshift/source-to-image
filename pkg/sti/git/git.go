package git

import (
	"bytes"
	"log"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/openshift/source-to-image/pkg/sti/util"
)

type Git interface {
	ValidCloneSpec(source string) bool
	Clone(source, target string) error
	Checkout(repo, ref string) error
}

type stiGit struct {
	verbose bool
	runner  util.CommandRunner
}

var gitSshUrlExp = regexp.MustCompile(`\A([\w\d\-_\.+]+@[\w\d\-_\.+]+:[\w\d\-_\.+%/]+\.git)$`)

var allowedSchemes = []string{"git", "http", "https", "file"}

func NewGit(verbose bool) Git {
	return &stiGit{
		verbose: verbose,
		runner:  util.NewCommandRunner(),
	}
}

func stringInSlice(s string, slice []string) bool {
	for _, element := range slice {
		if s == element {
			return true
		}
	}

	return false
}

func (h *stiGit) ValidCloneSpec(source string) bool {
	url, err := url.Parse(source)
	if err != nil {
		return false
	}

	if stringInSlice(url.Scheme, allowedSchemes) {
		return true
	}

	// support 'git@' ssh urls and local protocol without 'file://' scheme
	if url.Scheme == "" {
		if strings.HasSuffix(source, ".git") || (strings.HasPrefix(source, "git@") && gitSshUrlExp.MatchString(source)) {
			return true
		}
	}
	return false
}

func (h *stiGit) Clone(source, target string) error {
	opts := util.CommandOpts{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	if err := h.runner.RunWithOptions(opts, "git", "clone", "--recursive", source, target); err != nil {
		return err
	}
	return nil
}

func (h *stiGit) Checkout(repo, ref string) error {
	var buffer bytes.Buffer
	opts := util.CommandOpts{
		Stdout: &buffer,
		Stderr: &buffer,
		Dir:    repo,
	}
	if err := h.runner.RunWithOptions(opts, "git", "checkout", ref); err != nil {
		if h.verbose {
			log.Printf("Git checkout output:\n%s", buffer.String())
		}
		return err
	}
	return nil
}

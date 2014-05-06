package sti

import (
	"bytes"
	"log"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
)

var gitRefExp = regexp.MustCompile(`\A[\w\d\-_\.\^~]+$`)
var gitSshUrlExp = regexp.MustCompile(`\A([\w\d\-_\.+]+@[\w\d\-_\.+]+:[\w\d\-_\.+%/]+\.git)$`)

func validateGitRef(ref string) bool {
	return gitRefExp.MatchString(ref)
}

var allowedSchemes = []string{"git", "http", "https", "file"}

func validCloneSpec(source string, verbose bool) bool {
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

func gitClone(source, target string) (string, error) {
	var buffer bytes.Buffer
	cmd := exec.Command("git", "clone", "--recursive", source, target)
	cmd.Stdout, cmd.Stderr = &buffer, &buffer

	if err := cmd.Start(); err != nil {
		return buffer.String(), err
	}

	if err := cmd.Wait(); err != nil {
		return buffer.String(), err
	}

	return buffer.String(), nil
}

func gitCheckout(repo, ref string, verbose bool) error {
	var buffer bytes.Buffer
	cmd := exec.Command("git", "checkout", ref)
	cmd.Stdout, cmd.Stderr = &buffer, &buffer
	cmd.Dir = repo

	err := cmd.Start()
	if err != nil {
		return err
	}

	err = cmd.Wait()
	if err != nil {
		if verbose {
			log.Printf("Git checkout output:\n%s", buffer.String())
		}
		return err
	}

	return nil
}

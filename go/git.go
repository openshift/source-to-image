package sti

import (
	"bytes"
	"log"
	"os/exec"
	"regexp"
)

var gitRefExp = regexp.MustCompile(`\A[\w\d\-_\.\^~]+$`)

func validateGitRef(ref string) bool {
	return gitRefExp.MatchString(ref)
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

func gitCheckout(repo, ref string, debug bool) error {
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
		if debug {
			log.Printf("Git checkout output:\n%s", buffer.String())
		}
		return err
	}

	return nil
}

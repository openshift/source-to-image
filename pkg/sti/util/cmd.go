package util

import (
	"io"
	"os/exec"
)

type CommandOpts struct {
	Stdout io.Writer
	Stderr io.Writer
	Dir    string
}

type CommandRunner interface {
	RunWithOptions(opts CommandOpts, name string, arg ...string) error
	Run(name string, arg ...string) error
}

func NewCommandRunner() CommandRunner {
	return &runner{}
}

type runner struct{}

func (c *runner) RunWithOptions(opts CommandOpts, name string, arg ...string) error {
	cmd := exec.Command(name, arg...)
	if opts.Stdout != nil {
		cmd.Stdout = opts.Stdout
	}
	if opts.Stderr != nil {
		cmd.Stderr = opts.Stderr
	}
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	return cmd.Run()
}

func (c *runner) Run(name string, arg ...string) error {
	return c.RunWithOptions(CommandOpts{}, name, arg...)
}

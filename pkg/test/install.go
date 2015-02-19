package test

import (
	// "errors"

	"github.com/openshift/source-to-image/pkg/api"
)

// FakeInstaller provides a fake installer
type FakeInstaller struct {
	Scripts [][]api.Script
	DstDir  []string
	Error   error
}

func (f *FakeInstaller) run(scripts []api.Script, dstDir string) []api.InstallResult {
	result := []api.InstallResult{}
	f.Scripts = append(f.Scripts, scripts)
	f.DstDir = append(f.DstDir, dstDir)
	return result
}

func (f *FakeInstaller) InstallRequired(scripts []api.Script, dstDir string) ([]api.InstallResult, error) {
	return f.run(scripts, dstDir), f.Error
}

func (f *FakeInstaller) InstallOptional(scripts []api.Script, dstDir string) []api.InstallResult {
	return f.run(scripts, dstDir)
}

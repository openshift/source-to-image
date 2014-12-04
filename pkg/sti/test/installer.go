package test

// FakeInstaller provides a fake installer
type FakeInstaller struct {
	Scripts    [][]string
	WorkingDir []string
	Required   []bool

	Err error
}

// DownloadAndInstall downloads and install the fake STI scripts
func (f *FakeInstaller) DownloadAndInstall(scripts []string, workingDir string, required bool) error {
	f.Scripts = append(f.Scripts, scripts)
	f.WorkingDir = append(f.WorkingDir, workingDir)
	f.Required = append(f.Required, required)
	return f.Err
}

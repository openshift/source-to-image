package script

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/openshift/source-to-image/pkg/sti/test"
)

type FakeScriptHelperInternal struct {
	DownloadScripts     []string
	DownloadWorkingDir  string
	DownloadError       error
	DetermineScript     string
	DetermineWorkingDir string
	DetermineResult     map[string]string
	InstallScriptPath   string
	InstallWorkingDir   string
	InstallError        error
}

func (f *FakeScriptHelperInternal) downloadScripts(scripts []string, workingDir string) error {
	f.DownloadScripts = scripts
	f.DownloadWorkingDir = workingDir
	return f.DownloadError
}

func (f *FakeScriptHelperInternal) determineScriptPath(script string, workingDir string) string {
	f.DetermineScript = script
	f.DetermineWorkingDir = workingDir
	return f.DetermineResult[script]
}

func (f *FakeScriptHelperInternal) installScript(scriptPath string, workingDir string) error {
	f.InstallScriptPath = scriptPath
	f.InstallWorkingDir = workingDir
	return f.InstallError
}

func TestDownloadAndInstallScripts(t *testing.T) {
	type test struct {
		internal    FakeScriptHelperInternal
		required    bool
		errExpected bool
	}
	err := fmt.Errorf("Error")
	tests := map[string]test{
		"successRequired": {
			internal: FakeScriptHelperInternal{
				DetermineResult: map[string]string{
					"one":   "one",
					"two":   "two",
					"three": "three",
				},
			},
			required:    true,
			errExpected: false,
		},
		"successOptional": {
			internal: FakeScriptHelperInternal{
				DetermineResult: map[string]string{
					"one":   "one",
					"two":   "",
					"three": "three",
				},
			},
			required:    false,
			errExpected: false,
		},
		"downloadError": {
			internal: FakeScriptHelperInternal{
				DownloadError: err,
			},
			required:    true,
			errExpected: true,
		},
		"errorRequired": {
			internal: FakeScriptHelperInternal{
				DetermineResult: map[string]string{
					"one":   "one",
					"two":   "two",
					"three": "",
				},
			},
			required:    true,
			errExpected: true,
		},
		"installError": {
			internal: FakeScriptHelperInternal{
				DetermineResult: map[string]string{
					"one":   "one",
					"two":   "two",
					"three": "three",
				},
				InstallError: err,
			},
			required:    true,
			errExpected: true,
		},
	}

	for desc, test := range tests {
		sh := &installer{
			internal: &test.internal,
		}
		err := sh.DownloadAndInstall([]string{"one", "two", "three"}, "/test-working-dir", test.required)
		if !test.errExpected && err != nil {
			t.Errorf("%s: Unexpected error: %v", desc, err)
		} else if test.errExpected && err == nil {
			t.Errorf("%s: Error expected. Got nil.")
		}
		if !reflect.DeepEqual(sh.internal.(*FakeScriptHelperInternal).DownloadScripts,
			[]string{"one", "two", "three"}) {
			t.Errorf("%s: Unexpected downwload scripts: %#v",
				sh.internal.(*FakeScriptHelperInternal).DownloadScripts)
		}
	}
}

func getScriptInstaller() *installer {
	return &installer{
		verbose:    true,
		docker:     &test.FakeDocker{},
		image:      "test-image",
		scriptsUrl: "http://the.scripts.url/scripts",
		downloader: &test.FakeDownloader{},
		fs:         &test.FakeFileSystem{},
	}
}

func equalArrayContents(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, e := range a {
		found := false
		for _, f := range b {
			if f == e {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func TestDownloadScripts(t *testing.T) {
	sh := getScriptInstaller()
	dl := sh.downloader.(*test.FakeDownloader)
	sh.docker.(*test.FakeDocker).DefaultUrlResult = "http://image.url/scripts"
	err := sh.downloadScripts([]string{"one", "two", "three"}, "/working-dir")
	if err != nil {
		t.Errorf("Got unexpected error: %v", err)
	}
	if len(dl.URL) != 6 {
		t.Errorf("DownloadFile not called the expected number of times: %d",
			len(dl.URL))
	}
	expectedUrls := []string{
		"http://the.scripts.url/scripts/one",
		"http://the.scripts.url/scripts/two",
		"http://the.scripts.url/scripts/three",
		"http://image.url/scripts/one",
		"http://image.url/scripts/two",
		"http://image.url/scripts/three",
	}
	actualUrls := []string{}
	for _, u := range dl.URL {
		actualUrls = append(actualUrls, u.String())
	}
	if !equalArrayContents(actualUrls, expectedUrls) {
		t.Errorf("Unexpected set of URLs downloaded: %#v", actualUrls)
	}

	expectedFiles := []string{
		"/working-dir/downloads/scripts/one",
		"/working-dir/downloads/scripts/two",
		"/working-dir/downloads/scripts/three",
		"/working-dir/downloads/defaultScripts/one",
		"/working-dir/downloads/defaultScripts/two",
		"/working-dir/downloads/defaultScripts/three",
	}

	if !equalArrayContents(dl.File, expectedFiles) {
		t.Errorf("Unexpected set of files downloaded: %#v", dl.File)
	}
}

func TestDownloadScriptsDownloadErrors1(t *testing.T) {
	sh := getScriptInstaller()
	dl := sh.downloader.(*test.FakeDownloader)
	sh.docker.(*test.FakeDocker).DefaultUrlResult = "http://image.url/scripts"
	dlErr := fmt.Errorf("Download Error")
	dl.Err = map[string]error{
		"http://the.scripts.url/scripts/one":   dlErr,
		"http://the.scripts.url/scripts/two":   nil,
		"http://the.scripts.url/scripts/three": dlErr,
		"http://image.url/scripts/one":         nil,
		"http://image.url/scripts/two":         dlErr,
		"http://image.url/scripts/three":       nil,
	}
	err := sh.downloadScripts([]string{"one", "two", "three"}, "/working-dir")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestDownloadScriptsDownloadErrors2(t *testing.T) {
	sh := getScriptInstaller()
	dl := sh.downloader.(*test.FakeDownloader)
	sh.docker.(*test.FakeDocker).DefaultUrlResult = "http://image.url/scripts"
	dlErr := fmt.Errorf("Download Error")
	dl.Err = map[string]error{
		"http://the.scripts.url/scripts/one":   dlErr,
		"http://the.scripts.url/scripts/two":   nil,
		"http://the.scripts.url/scripts/three": nil,
		"http://image.url/scripts/one":         dlErr,
		"http://image.url/scripts/two":         dlErr,
		"http://image.url/scripts/three":       nil,
	}
	err := sh.downloadScripts([]string{"one", "two", "three"}, "/working-dir")
	if err == nil {
		t.Errorf("Expected an error because script could not be downloaded")
	}
}

func TestDownloadScriptsChmodError(t *testing.T) {
	sh := getScriptInstaller()
	fsErr := fmt.Errorf("Chmod Error")
	sh.docker.(*test.FakeDocker).DefaultUrlResult = "http://image.url/scripts"
	sh.fs.(*test.FakeFileSystem).ChmodError = map[string]error{
		"/working-dir/downloads/scripts/one":          nil,
		"/working-dir/downloads/scripts/two":          nil,
		"/working-dir/downloads/scripts/three":        fsErr,
		"/working-dir/downloads/defaultScripts/one":   nil,
		"/working-dir/downloads/defaultScripts/two":   nil,
		"/working-dir/downloads/defaultScripts/three": nil,
	}
	err := sh.downloadScripts([]string{"one", "two", "three"}, "/working-dir")
	if err == nil {
		t.Errorf("Expected an error because chmod returned an error.")
	}
}

func TestDetermineScriptPath(t *testing.T) {
	sh := getScriptInstaller()
	fs := sh.fs.(*test.FakeFileSystem)

	fs.ExistsResult = map[string]bool{"/working-dir/downloads/defaultScripts/script1": true}
	workingDir := "/working-dir"
	path := sh.determineScriptPath("script1", workingDir)

	if path != "/working-dir/downloads/defaultScripts/script1" {
		t.Errorf("Unexpected path result: %s", path)
	}
}

func TestInstallScript(t *testing.T) {
	sh := getScriptInstaller()
	fs := sh.fs.(*test.FakeFileSystem)
	scriptPath := "/working-dir/downloads/scripts/test1"

	err := sh.installScript(scriptPath, "/working-dir")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if fs.RenameFrom != scriptPath {
		t.Errorf("Unexpected rename source: %s", fs.RenameFrom)
	}
	if fs.RenameTo != "/working-dir/upload/scripts/test1" {
		t.Errorf("Unexpected rename destination: %s", fs.RenameTo)
	}
}

func TestPrepareScriptDownload(t *testing.T) {
	sh := getScriptInstaller()
	result := sh.prepareScriptDownload(
		[]string{"test1", "test2"},
		"/working-dir/upload",
		"http://my.url/base")

	for k, v := range result {
		if v.name == "test1" {
			if v.url.String() != "http://my.url/base/test1" {
				t.Errorf("Unexpected URL: %s", v.url)
			}
			if k != "/working-dir/upload/test1" {
				t.Errorf("Unexpected directory: %s", v.url)
			}

		} else if v.name == "test2" {
			if v.url.String() != "http://my.url/base/test2" {
				t.Errorf("Unexpected URL: %s", v.url)
			}
			if k != "/working-dir/upload/test2" {
				t.Errorf("Unexpected directory: %s", v.url)
			}
		}
	}
}

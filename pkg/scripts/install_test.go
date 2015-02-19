package scripts

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/errors"
	"github.com/openshift/source-to-image/pkg/test"
)

func getFakeInstaller() *installer {
	return &installer{
		image:      "test-image",
		scriptsURL: "http://the.scripts.url/scripts",
		docker:     &test.FakeDocker{},
		downloader: &test.FakeDownloader{},
		fs:         &test.FakeFileSystem{},
	}
}

func TestInstallRequiredError(t *testing.T) {
	inst := getFakeInstaller()
	inst.downloader.(*test.FakeDownloader).Err = map[string]error{
		inst.scriptsURL + "/" + string(api.Assemble): fmt.Errorf("Download Error"),
	}

	_, err := inst.InstallRequired([]api.Script{api.Assemble, api.Run}, "/working-dir/")
	if err == nil {
		t.Error("Expected error but none got!")
	}
}

func TestRun(t *testing.T) {
	inst := getFakeInstaller()
	defaultURL := "http://the.default.url"
	inst.docker.(*test.FakeDocker).DefaultURLResult = defaultURL
	scriptsURL := "http://the.scripts.url"
	inst.scriptsURL = scriptsURL
	workingDir := "/working-dir/"
	fs := inst.fs.(*test.FakeFileSystem)
	fs.ExistsResult = map[string]bool{
		filepath.Join(workingDir, api.UserScripts, string(api.Assemble)):    true,
		filepath.Join(workingDir, api.UserScripts, string(api.Run)):         true,
		filepath.Join(workingDir, api.DefaultScripts, string(api.Assemble)): true,
		filepath.Join(workingDir, api.DefaultScripts, string(api.Run)):      true,
	}

	result := inst.run([]api.Script{api.Assemble, api.Run}, workingDir)
	if len(result) != 2 {
		t.Errorf("Unexpected result length, expected 2, got %d", len(result))
	}
	for _, r := range result {
		if r.Error != nil {
			t.Errorf("Unexpected error run for %v: %v", r.Script, r.Error)
		}
		if !r.Downloaded {
			t.Errorf("%v was not downloaded", r.Script)
		}
		if !r.Installed {
			t.Errorf("%v was not installed", r.Script)
		}
	}
}

func TestRunNoDefaultURL(t *testing.T) {
	inst := getFakeInstaller()
	scriptsURL := "http://the.scripts.url"
	inst.scriptsURL = scriptsURL
	workingDir := "/working-dir/"
	fs := inst.fs.(*test.FakeFileSystem)
	fs.ExistsResult = map[string]bool{
		filepath.Join(workingDir, api.UserScripts, string(api.Assemble)): true,
		filepath.Join(workingDir, api.UserScripts, string(api.Run)):      true,
	}

	result := inst.run([]api.Script{api.Assemble, api.Run}, workingDir)
	if len(result) != 2 {
		t.Errorf("Unexpected result length, expected 2, got %d", len(result))
	}
	for _, r := range result {
		if r.Error != nil {
			t.Errorf("Unexpected error run for %v: %v", r.Script, r.Error)
		}
		if !r.Downloaded {
			t.Errorf("%v was not downloaded", r.Downloaded)
		}
		if !r.Installed {
			t.Errorf("%v was not installed", r.Installed)
		}
	}
}

func TestRunEmpty(t *testing.T) {
	inst := getFakeInstaller()
	result := inst.run([]api.Script{}, "")
	if result == nil || len(result) != 0 {
		t.Error("Unexpected result from run!")
	}
}

func TestDownloadErrors(t *testing.T) {
	inst := getFakeInstaller()
	baseURL := "http://the.scripts.url"
	dl := inst.downloader.(*test.FakeDownloader)
	dlErr := fmt.Errorf("Download Error")
	dl.Err = map[string]error{
		baseURL + "/" + string(api.Assemble):      dlErr,
		baseURL + "/" + string(api.Run):           nil,
		baseURL + "/" + string(api.SaveArtifacts): dlErr,
	}

	result := inst.download(baseURL, []api.Script{api.Assemble, api.Run, api.SaveArtifacts}, "")
	for s, r := range result {
		e := dl.Err[baseURL+"/"+string(s)]
		a := r.err
		if e != a {
			t.Errorf("Expected download error '%v' for %v, but got %v", e, s, a)
		}
	}
}

func TestInstallFromDefaultURL(t *testing.T) {
	defaultURL := "http://the.default.url"
	defaultResults := map[api.Script]*downloadResult{
		api.Assemble: {defaultURL, nil},
		api.Run:      {defaultURL, nil},
	}

	testInstall(t, getFakeInstaller(), []api.Script{api.Assemble, api.Run},
		nil, nil, defaultResults, "/working-dir/",
		defaultURL, true, true, nil)
}

func TestInstallFromScriptsURL(t *testing.T) {
	scriptsURL := "http://the.scripts.url"
	userResults := map[api.Script]*downloadResult{
		api.Assemble: {scriptsURL, nil},
		api.Run:      {scriptsURL, nil},
	}

	defaultURL := "http://the.default.url"
	defaultResults := map[api.Script]*downloadResult{
		api.Assemble: {defaultURL, nil},
		api.Run:      {defaultURL, nil},
	}

	testInstall(t, getFakeInstaller(), []api.Script{api.Assemble, api.Run},
		userResults, nil, defaultResults, "/working-dir/",
		scriptsURL, true, true, nil)
}

func TestInstallFromSourceURL(t *testing.T) {
	sourceResults := map[api.Script]*downloadResult{
		api.Assemble: {api.SourceScripts, nil},
		api.Run:      {api.SourceScripts, nil},
	}

	defaultURL := "http://the.default.url"
	defaultResults := map[api.Script]*downloadResult{
		api.Assemble: {defaultURL, nil},
		api.Run:      {defaultURL, nil},
	}

	testInstall(t, getFakeInstaller(), []api.Script{api.Assemble, api.Run},
		nil, sourceResults, defaultResults, "/working-dir/",
		api.SourceScripts, true, true, nil)
}

func TestInstallScriptsFromImage(t *testing.T) {
	defaultURL := "image:///path/in/image"
	defaultResults := map[api.Script]*downloadResult{
		api.Assemble: {defaultURL, errors.NewScriptsInsideImageError(defaultURL)},
		api.Run:      {defaultURL, errors.NewScriptsInsideImageError(defaultURL)},
	}

	testInstall(t, getFakeInstaller(), []api.Script{api.Assemble, api.Run},
		nil, nil, defaultResults, "/working-dir/",
		defaultURL, false, true, nil)
}

func TestInstallJustErrors(t *testing.T) {
	err1 := fmt.Errorf("Just errors")
	scriptsURL := "http://the.scripts.url"
	userResults := map[api.Script]*downloadResult{
		api.Assemble: {scriptsURL, err1},
		api.Run:      {scriptsURL, err1},
	}

	err2 := fmt.Errorf("Just errors")
	defaultURL := "image:///path/in/image"
	defaultResults := map[api.Script]*downloadResult{
		api.Assemble: {defaultURL, err2},
		api.Run:      {defaultURL, err2},
	}

	testInstall(t, getFakeInstaller(), []api.Script{api.Assemble, api.Run},
		userResults, nil, defaultResults, "/working-dir/",
		defaultURL, false, false, err2)
}

func TestInstallEmpty(t *testing.T) {
	testInstall(t, getFakeInstaller(), []api.Script{api.Assemble, api.Run},
		nil, nil, nil, "/working-dir/",
		"", false, false, nil)
}

func TestInstallRenameErr(t *testing.T) {
	inst := getFakeInstaller()
	fsErr := fmt.Errorf("Rename Error")
	inst.fs.(*test.FakeFileSystem).RenameError = fsErr

	defaultURL := "http://the.default.url"
	defaultResults := map[api.Script]*downloadResult{
		api.Assemble: {defaultURL, nil},
		api.Run:      {defaultURL, nil},
	}

	testInstall(t, inst, []api.Script{api.Assemble, api.Run},
		nil, nil, defaultResults, "/working-dir/",
		defaultURL, false, false, fsErr)
}

func TestInstallChmodErr(t *testing.T) {
	inst := getFakeInstaller()
	workingDir := "/working-dir/"
	fsErr := fmt.Errorf("Chmod Error")
	inst.fs.(*test.FakeFileSystem).ChmodError = map[string]error{
		filepath.Join(workingDir, api.UploadScripts, string(api.Assemble)): fsErr,
		filepath.Join(workingDir, api.UploadScripts, string(api.Run)):      fsErr,
	}

	defaultURL := "http://the.default.url"
	defaultResults := map[api.Script]*downloadResult{
		api.Assemble: {defaultURL, nil},
		api.Run:      {defaultURL, nil},
	}

	testInstall(t, inst, []api.Script{api.Assemble, api.Run},
		nil, nil, defaultResults, workingDir,
		defaultURL, false, false, fsErr)
}

func testInstall(t *testing.T, inst *installer, scripts []api.Script, userResults, sourceResults,
	defaultResults map[api.Script]*downloadResult, workingDir, expectedURL string,
	expectedDownloaded, expectedInstalled bool, expectedError error) {
	result := inst.install(scripts, userResults, sourceResults, defaultResults, workingDir)

	if len(result) != len(scripts) {
		t.Errorf("Unexpected result length, expected %d, got %d", len(scripts), len(result))
	}
	for _, r := range result {
		if r.Error != expectedError {
			t.Errorf("Unexpected error during install %s, expected %v, got %v", r.Script, expectedError, r.Error)
		}
		if r.URL != expectedURL {
			t.Errorf("Unexpected location for %s, expected %s, got %s", r.Script, expectedURL, r.URL)
		}
		if r.Downloaded != expectedDownloaded {
			t.Errorf("Unexpected download flag for %s, got %v, expected %v", r.Script, expectedDownloaded, r.Downloaded)
		}
		if r.Installed != expectedInstalled {
			t.Errorf("Unexpected download flag for %s, got %v, expected %v", r.Script, expectedInstalled, r.Installed)
		}
	}
}

func TestInstallCombined(t *testing.T) {
	scriptsURL := "http://the.scripts.url"
	userResults := map[api.Script]*downloadResult{
		api.Assemble: {scriptsURL, nil},
	}

	sourceResults := map[api.Script]*downloadResult{
		api.Run: {api.SourceScripts, nil},
	}

	defaultURL := "image:///path/in/image"
	defaultResults := map[api.Script]*downloadResult{
		api.Assemble:      {defaultURL, errors.NewScriptsInsideImageError(defaultURL)},
		api.Run:           {defaultURL, errors.NewScriptsInsideImageError(defaultURL)},
		api.SaveArtifacts: {defaultURL, errors.NewScriptsInsideImageError(defaultURL)},
	}

	inst := getFakeInstaller()
	result := inst.install([]api.Script{api.Assemble, api.Run, api.SaveArtifacts}, userResults, sourceResults, defaultResults, "/working-dir/")

	if len(result) != 3 {
		t.Errorf("Unexpected result length, expected 3, got %d", len(result))
	}
	for _, r := range result {
		if r.Error != nil {
			t.Errorf("Unexpected error during install %s, got %v", r.Script, r.Error)
		}
		switch r.Script {
		case api.Assemble:
			if r.URL != scriptsURL || !r.Downloaded || !r.Installed {
				t.Errorf("Unexpected results for %s: %s, %v, %v", r.Script, r.URL, r.Downloaded, r.Installed)
			}
		case api.Run:
			if r.URL != api.SourceScripts || !r.Downloaded || !r.Installed {
				t.Errorf("Unexpected results for %s: %s, %v, %v", r.Script, r.URL, r.Downloaded, r.Installed)
			}
		case api.SaveArtifacts:
			if r.URL != defaultURL || r.Downloaded || !r.Installed {
				t.Errorf("Unexpected results for %s: %s, %v, %v", r.Script, r.URL, r.Downloaded, r.Installed)
			}
		}
	}
}

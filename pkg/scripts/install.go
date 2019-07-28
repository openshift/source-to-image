package scripts

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/api/constants"
	"github.com/openshift/source-to-image/pkg/docker"
	s2ierr "github.com/openshift/source-to-image/pkg/errors"
	"github.com/openshift/source-to-image/pkg/util/fs"
)

// Installer interface is responsible for installing scripts needed to run the
// build.
type Installer interface {
	InstallRequired(scripts []string, dstDir string) ([]api.InstallResult, error)
	InstallOptional(scripts []string, dstDir string) []api.InstallResult
}

// ScriptHandler provides an interface for various scripts source handlers.
type ScriptHandler interface {
	Get(script string) *api.InstallResult
	Install(*api.InstallResult) error
	SetDestinationDir(string)
	String() string
}

// URLScriptHandler handles script download using URL.
type URLScriptHandler struct {
	URL            string
	DestinationDir string
	Download       Downloader
	FS             fs.FileSystem
	Name           string
}

const (
	sourcesRootAbbrev = "<source-dir>"

	// ScriptURLHandler is the name of the script URL handler
	ScriptURLHandler = "script URL handler"

	// ImageURLHandler is the name of the image URL handler
	ImageURLHandler = "image URL handler"

	// SourceHandler is the name of the source script handler
	SourceHandler = "source handler"
)

var (
	// RequiredScripts must be present to do an s2i build
	RequiredScripts = []string{constants.Assemble, constants.Run}

	// OptionalScripts may be provided when doing an s2i build
	OptionalScripts = []string{constants.SaveArtifacts}
)

// SetDestinationDir sets the destination where the scripts should be
// downloaded.
func (s *URLScriptHandler) SetDestinationDir(baseDir string) {
	s.DestinationDir = baseDir
}

// String implements the String() function.
func (s *URLScriptHandler) String() string {
	return s.Name
}

// Get parses the provided URL and the script name.
func (s *URLScriptHandler) Get(script string) *api.InstallResult {
	if len(s.URL) == 0 {
		return nil
	}
	scriptURL, err := url.ParseRequestURI(s.URL + "/" + script)
	if err != nil {
		log.Infof("invalid script url %q: %v", s.URL, err)
		return nil
	}
	return &api.InstallResult{
		Script: script,
		URL:    scriptURL.String(),
	}
}

// Install downloads the script and fix its permissions.
func (s *URLScriptHandler) Install(r *api.InstallResult) error {
	downloadURL, err := url.Parse(r.URL)
	if err != nil {
		return err
	}
	dst := filepath.Join(s.DestinationDir, constants.UploadScripts, r.Script)
	if _, err := s.Download.Download(downloadURL, dst); err != nil {
		if e, ok := err.(s2ierr.Error); ok {
			if e.ErrorCode == s2ierr.ScriptsInsideImageError {
				r.Installed = true
				return nil
			}
		}
		return err
	}
	if err := s.FS.Chmod(dst, 0755); err != nil {
		return err
	}
	r.Installed = true
	r.Downloaded = true
	return nil
}

// SourceScriptHandler handles the case when the scripts are contained in the
// source code directory.
type SourceScriptHandler struct {
	DestinationDir string
	fs             fs.FileSystem
}

// Get verifies if the script is present in the source directory and get the
// installation result.
func (s *SourceScriptHandler) Get(script string) *api.InstallResult {
	location := filepath.Join(s.DestinationDir, constants.SourceScripts, script)
	if s.fs.Exists(location) {
		return &api.InstallResult{Script: script, URL: location}
	}
	// TODO: The '.sti/bin' path inside the source code directory is deprecated
	// and this should (and will) be removed soon.
	location = filepath.FromSlash(strings.Replace(filepath.ToSlash(location), "s2i/bin", "sti/bin", 1))
	if s.fs.Exists(location) {
		log.Info("DEPRECATED: Use .s2i/bin instead of .sti/bin")
		return &api.InstallResult{Script: script, URL: location}
	}
	return nil
}

// String implements the String() function.
func (s *SourceScriptHandler) String() string {
	return SourceHandler
}

// Install copies the script into upload directory and fix its permissions.
func (s *SourceScriptHandler) Install(r *api.InstallResult) error {
	dst := filepath.Join(s.DestinationDir, constants.UploadScripts, r.Script)
	if err := s.fs.Rename(r.URL, dst); err != nil {
		return err
	}
	if err := s.fs.Chmod(dst, 0755); err != nil {
		return err
	}
	// Make the path to scripts nicer in logs
	parts := strings.Split(filepath.ToSlash(r.URL), "/")
	if len(parts) > 3 {
		r.URL = filepath.FromSlash(sourcesRootAbbrev + "/" + strings.Join(parts[len(parts)-3:], "/"))
	}
	r.Installed = true
	r.Downloaded = true
	return nil
}

// SetDestinationDir sets the directory where the scripts should be uploaded.
// In case of SourceScriptHandler this is a source directory root.
func (s *SourceScriptHandler) SetDestinationDir(baseDir string) {
	s.DestinationDir = baseDir
}

// ScriptSourceManager manages various script handlers.
type ScriptSourceManager interface {
	Add(ScriptHandler)
	SetDownloader(Downloader)
	Installer
}

// DefaultScriptSourceManager manages the default script lookup and installation
// for source-to-image.
type DefaultScriptSourceManager struct {
	Image      string
	ScriptsURL string
	download   Downloader
	docker     docker.Docker
	dockerAuth api.AuthConfig
	sources    []ScriptHandler
	fs         fs.FileSystem
}

// Add registers a new script source handler.
func (m *DefaultScriptSourceManager) Add(s ScriptHandler) {
	if len(m.sources) == 0 {
		m.sources = []ScriptHandler{}
	}
	m.sources = append(m.sources, s)
}

// NewInstaller returns a new instance of the default Installer implementation
func NewInstaller(image string, scriptsURL string, proxyConfig *api.ProxyConfig, docker docker.Docker, auth api.AuthConfig, fs fs.FileSystem) Installer {
	m := DefaultScriptSourceManager{
		Image:      image,
		ScriptsURL: scriptsURL,
		dockerAuth: auth,
		docker:     docker,
		fs:         fs,
		download:   NewDownloader(proxyConfig),
	}
	// Order is important here, first we try to get the scripts from provided URL,
	// then we look into sources and check for .s2i/bin scripts.
	if len(m.ScriptsURL) > 0 {
		m.Add(&URLScriptHandler{URL: m.ScriptsURL, Download: m.download, FS: m.fs, Name: ScriptURLHandler})
	}

	m.Add(&SourceScriptHandler{fs: m.fs})

	if m.docker != nil {
		// If the detection handlers above fail, try to get the script url from the
		// docker image itself.
		defaultURL, err := m.docker.GetScriptsURL(m.Image)
		if err == nil && defaultURL != "" {
			m.Add(&URLScriptHandler{URL: defaultURL, Download: m.download, FS: m.fs, Name: ImageURLHandler})
		}
	}
	return &m
}

// InstallRequired Downloads and installs required scripts into dstDir, the result is a
// map of scripts with detailed information about each of the scripts install process
// with error if installing some of them failed
func (m *DefaultScriptSourceManager) InstallRequired(scripts []string, dstDir string) ([]api.InstallResult, error) {
	result := m.InstallOptional(scripts, dstDir)
	failedScripts := []string{}
	var err error
	for _, r := range result {
		if r.Error != nil {
			failedScripts = append(failedScripts, r.Script)
		}
	}
	if len(failedScripts) > 0 {
		err = s2ierr.NewInstallRequiredError(failedScripts, constants.ScriptsURLLabel)
	}
	return result, err
}

// InstallOptional downloads and installs a set of scripts into dstDir, the result is a
// map of scripts with detailed information about each of the scripts install process
func (m *DefaultScriptSourceManager) InstallOptional(scripts []string, dstDir string) []api.InstallResult {
	result := []api.InstallResult{}
	for _, script := range scripts {
		installed := false
		failedSources := []string{}
		for _, e := range m.sources {
			detected := false
			h := e.(ScriptHandler)
			h.SetDestinationDir(dstDir)
			if r := h.Get(script); r != nil {
				if err := h.Install(r); err != nil {
					failedSources = append(failedSources, h.String())
					// all this means is this source didn't have this particular script
					log.V(4).Infof("script %q found by the %s, but failed to install: %v", script, h, err)
				} else {
					r.FailedSources = failedSources
					result = append(result, *r)
					installed = true
					detected = true
					log.V(4).Infof("Using %q installed from %q", script, r.URL)
				}
			}
			if detected {
				break
			}
		}
		if !installed {
			result = append(result, api.InstallResult{
				FailedSources: failedSources,
				Script:        script,
				Error:         fmt.Errorf("script %q not installed", script),
			})
		}
	}
	return result
}

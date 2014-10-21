package script

import (
	"fmt"
	"log"
	"net/url"
	"path/filepath"
	"sync"

	"github.com/openshift/source-to-image/pkg/sti/docker"
	"github.com/openshift/source-to-image/pkg/sti/errors"
	"github.com/openshift/source-to-image/pkg/sti/util"
)

type installer struct {
	verbose    bool
	docker     docker.Docker
	image      string
	scriptsUrl string
	downloader util.Downloader
	fs         util.FileSystem
	internal   installerInternal
}

type Installer interface {
	DownloadAndInstall(scripts []string, workingDir string, required bool) error
}

func NewInstaller(image, scriptsUrl string, docker docker.Docker, verbose bool) Installer {
	i := installer{
		verbose:    verbose,
		image:      image,
		scriptsUrl: scriptsUrl,
		docker:     docker,
		downloader: util.NewDownloader(verbose),
		fs:         util.NewFileSystem(verbose),
	}
	i.internal = &i
	return &i
}

type installerInternal interface {
	downloadScripts(scripts []string, workingDir string) error
	determineScriptPath(script string, workingDir string) string
	installScript(scriptPath string, workingDir string) error
}

type scriptInfo struct {
	url  *url.URL
	name string
}

func (s *installer) DownloadAndInstall(scripts []string, workingDir string, required bool) error {
	i := s.internal
	err := i.downloadScripts(scripts, workingDir)
	if err != nil {
		return err
	}

	for _, script := range scripts {
		scriptPath := i.determineScriptPath(script, workingDir)
		if required && scriptPath == "" {
			err = fmt.Errorf("No %s script found in provided url, application source, or default image url. Aborting.", script)
			return err
		}
		err = i.installScript(scriptPath, workingDir)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *installer) downloadScripts(scripts []string, workingDir string) error {
	if len(scripts) == 0 {
		return nil
	}

	var (
		wg        sync.WaitGroup
		errs      map[string]chan error
		downloads map[string]chan struct{}
	)

	errs = make(map[string]chan error)
	downloads = make(map[string]chan struct{})
	for _, s := range scripts {
		errs[s] = make(chan error, 2)
		downloads[s] = make(chan struct{}, 2)
	}

	downloadAsync := func(script string, scriptUrl *url.URL, targetFile string) {
		defer wg.Done()
		err := s.downloader.DownloadFile(scriptUrl, targetFile)
		if err != nil {
			return
		}
		downloads[script] <- struct{}{}

		err = s.fs.Chmod(targetFile, 0700)
		if err != nil {
			errs[script] <- err
		}
	}

	if s.scriptsUrl != "" {
		destDir := filepath.Join(workingDir, "/downloads/scripts")
		for file, info := range s.prepareScriptDownload(scripts, destDir, s.scriptsUrl) {
			wg.Add(1)
			go downloadAsync(info.name, info.url, file)
		}
	}

	defaultUrl, err := s.docker.GetDefaultUrl(s.image)
	if err != nil {
		return fmt.Errorf("Unable to retrieve the default STI scripts URL: %s", err.Error())
	}

	if defaultUrl != "" {
		destDir := filepath.Join(workingDir, "/downloads/defaultScripts")
		for file, info := range s.prepareScriptDownload(scripts, destDir, defaultUrl) {
			wg.Add(1)
			go downloadAsync(info.name, info.url, file)
		}
	}

	// Wait for the script downloads to finish.
	wg.Wait()
	for _, d := range downloads {
		if len(d) == 0 {
			return errors.ErrScriptsDownloadFailed
		}
	}

	for _, e := range errs {
		if len(e) > 0 {
			return errors.ErrScriptsDownloadFailed
		}
	}

	return nil
}

func (s *installer) determineScriptPath(script string, workingDir string) string {
	locations := []string{
		"downloads/scripts",
		"upload/src/.sti/bin",
		"downloads/defaultScripts",
	}
	descriptions := []string{
		"user provided url",
		"application source",
		"default url reference in the image",
	}

	for i, location := range locations {
		path := filepath.Join(workingDir, location, script)
		if s.verbose {
			log.Printf("Looking for %s script at %s", script, path)
		}
		if s.fs.Exists(path) {
			if s.verbose {
				log.Printf("Found %s script from %s.", script, descriptions[i])
			}
			return path
		}
	}

	return ""
}

func (s *installer) installScript(path string, workingDir string) error {
	script := filepath.Base(path)
	return s.fs.Rename(path, filepath.Join(workingDir, "upload/scripts", script))
}

// prepareScriptDownload turns the script name into proper URL
func (s *installer) prepareScriptDownload(scripts []string, targetDir, baseUrl string) map[string]scriptInfo {

	s.fs.MkdirAll(targetDir)

	info := make(map[string]scriptInfo)

	for _, script := range scripts {
		url, err := url.Parse(baseUrl + "/" + script)
		if err != nil {
			log.Printf("[WARN] Unable to parse script URL: %n\n", baseUrl+"/"+script)
			continue
		}

		info[targetDir+"/"+script] = scriptInfo{url, script}
	}

	return info
}

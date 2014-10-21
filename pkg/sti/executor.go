package sti

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/openshift/source-to-image/pkg/sti/docker"
	"github.com/openshift/source-to-image/pkg/sti/script"
	"github.com/openshift/source-to-image/pkg/sti/tar"
	"github.com/openshift/source-to-image/pkg/sti/util"
)

// STIRequest contains essential fields for any request: a Configuration, a base image, and an
// optional runtime image.
type STIRequest struct {
	BaseImage           string
	DockerSocket        string
	Verbose             bool
	PreserveWorkingDir  bool
	Source              string
	Ref                 string
	Tag                 string
	Clean               bool
	RemovePreviousImage bool
	Environment         map[string]string
	CallbackUrl         string
	ScriptsUrl          string

	incremental bool
	workingDir  string
}

type STIResult struct {
	Success    bool
	Messages   []string
	WorkingDir string
	ImageID    string
}

// requestHandler encapsulates dependencies needed to fulfill requests.
type requestHandler struct {
	request      *STIRequest
	result       *STIResult
	postExecutor postExecutor
	installer    script.Installer
	fs           util.FileSystem
	docker       docker.Docker
	tar          tar.Tar
}

const DefaultUntarTimeout = 2 * time.Minute

type postExecutor interface {
	postExecute(containerID string, cmd []string) error
}

// newReuestHandler returns a new handler for a given request.
func newRequestHandler(req *STIRequest) (*requestHandler, error) {
	if req.Verbose {
		log.Printf("Using docker socket: %s\n", req.DockerSocket)
	}

	docker, err := docker.NewDocker(req.DockerSocket, req.Verbose)
	if err != nil {
		return nil, err
	}

	return &requestHandler{
		request:   req,
		docker:    docker,
		installer: script.NewInstaller(req.BaseImage, req.ScriptsUrl, docker, req.Verbose),
		fs:        util.NewFileSystem(req.Verbose),
		tar:       tar.NewTar(req.Verbose),
	}, nil
}

func (h *requestHandler) setup(requiredScripts, optionalScripts []string) error {
	var err error
	h.request.workingDir, err = h.fs.CreateWorkingDirectory()
	if err != nil {
		return err
	}

	h.result = &STIResult{
		Success:    false,
		WorkingDir: h.request.workingDir,
	}

	dirs := []string{"upload/scripts", "downloads/scripts", "downloads/defaultScripts"}
	for _, v := range dirs {
		err := h.fs.MkdirAll(filepath.Join(h.request.workingDir, v))
		if err != nil {
			return err
		}
	}

	err = h.installer.DownloadAndInstall(requiredScripts, h.request.workingDir, true)
	if err != nil {
		return err
	}

	err = h.installer.DownloadAndInstall(optionalScripts, h.request.workingDir, false)
	if err != nil {
		return err
	}

	return nil
}

func (h *requestHandler) generateConfigEnv() []string {
	var configEnv []string
	if len(h.request.Environment) > 0 {
		for key, val := range h.request.Environment {
			configEnv = append(configEnv, key+"="+val)
		}
	}
	return configEnv
}

func (h *requestHandler) execute(command string) error {
	if h.request.Verbose {
		log.Printf("Using image name %s", h.request.BaseImage)
	}

	uploadDir := filepath.Join(h.request.workingDir, "upload")
	tarFileName, err := h.tar.CreateTarFile(h.request.workingDir, uploadDir)
	if err != nil {
		return err
	}

	tarFile, err := h.fs.Open(tarFileName)
	if err != nil {
		return err
	}
	defer tarFile.Close()

	opts := docker.RunContainerOptions{
		Image:     h.request.BaseImage,
		Stdin:     tarFile,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
		PullImage: true,
		Command:   command,
		Env:       h.generateConfigEnv(),
	}
	if h.postExecutor != nil {
		opts.PostExec = h.postExecutor.postExecute
	}
	err = h.docker.RunContainer(opts)

	return nil
}

func (h *requestHandler) cleanup() {
	if h.request.PreserveWorkingDir {
		log.Printf("Temporary directory '%s' will be saved, not deleted\n", h.request.workingDir)
	} else {
		h.fs.RemoveDirectory(h.request.workingDir)
	}
}

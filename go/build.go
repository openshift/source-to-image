package sti

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/fsouza/go-dockerclient"
)

type BuildRequest struct {
	Request
	Source      string
	Ref         string
	Tag         string
	Clean       bool
	Environment map[string]string
	Method      string
	Writer      io.Writer
	CallbackUrl string
}

type BuildResult STIResult

// Build processes a BuildRequest and returns a *BuildResult and an error.
// An error represents a failure performing the build rather than a failure
// of the build itself.  Callers should check the Success field of the result
// to determine whether a build succeeded or not.
func Build(req BuildRequest) (*BuildResult, error) {
	method := req.Method
	if method == "" {
		req.Method = "run"
	} else {
		if !stringInSlice(method, []string{"run", "build"}) {
			return nil, ErrInvalidBuildMethod
		}
	}

	h, err := newHandler(req.Request)
	if err != nil {
		return nil, err
	}

	tag := req.Tag
	incremental := !req.Clean

	if incremental {
		exists, err := h.isImageInLocalRegistry(tag)

		if err != nil {
			return nil, err
		}

		if exists {
			incremental, err = h.detectIncrementalBuild(tag)
			if err != nil {
				return nil, err
			}
		} else {
			incremental = false
		}
	}

	if incremental {
		log.Printf("Existing image for tag %s detected for incremental build\n", tag)
	} else {
		log.Println("Clean build will be performed")
	}

	var result *BuildResult

	result, err = h.build(req, incremental)

	if req.CallbackUrl != "" {
		executeCallback(req.CallbackUrl, result)
	}

	return result, err
}

func executeCallback(callbackUrl string, result *BuildResult) {

	buf := new(bytes.Buffer)
	writer := bufio.NewWriter(buf)
	for _, message := range result.Messages {
		fmt.Fprintln(writer, message)
	}
	writer.Flush()

	d := map[string]interface{}{
		"payload": buf.String(),
		"success": result.Success,
	}

	jsonBuffer := new(bytes.Buffer)
	writer = bufio.NewWriter(jsonBuffer)
	jsonWriter := json.NewEncoder(writer)
	jsonWriter.Encode(d)
	writer.Flush()

	var resp *http.Response
	var err error

	for retries := 0; retries < 3; retries++ {
		resp, err = http.Post(callbackUrl, "application/json", jsonBuffer)
		if err != nil {
			errorMessage := fmt.Sprintf("Unable to invoke callback: %s", err.Error())
			result.Messages = append(result.Messages, errorMessage)
		}
		if resp != nil {
			if resp.StatusCode >= 300 {
				errorMessage := fmt.Sprintf("Callback returned with error code: %d", resp.StatusCode)
				result.Messages = append(result.Messages, errorMessage)
			}
			break
		}
	}
}

// Script used to initialize permissions on bind-mounts when a non-root user is specified by an image
var saveArtifactsInitTemplate = template.Must(template.New("sa-init.sh").Parse(`#!/bin/bash
chown -R {{.User}}:{{.User}} /tmp/artifacts && chmod -R 755 /tmp/artifacts
exec su {{.User}} -s /bin/bash -c /usr/bin/save-artifacts
`))

// Script used to initialize permissions on bind-mounts for a docker-run build (prepare call)
var buildTemplate = template.Must(template.New("build-init.sh").Parse(`#!/bin/bash
chown -R {{.User}}:{{.User}} /tmp/src && chmod -R 755 /tmp/src
{{if .Incremental}}chown -R {{.User}}:{{.User}} /tmp/artifacts && chmod -R 755 /tmp/artifacts{{end}}
exec su {{.User}} -s /bin/bash -c /usr/bin/prepare
`))

var dockerFileTemplate = template.Must(template.New("Dockerfile").Parse(`
FROM {{.BaseImage}}
{{if .User}}USER root{{end}}
ADD ./src /tmp/src/
{{if .User}}RUN chown -R {{.User}}:{{.User}} /tmp/src && chmod -R 755 /tmp/src{{end}}
{{if .Incremental}}
ADD ./artifacts /tmp/artifacts
{{if .User}}RUN chown -R {{.User}}:{{.User}} /tmp/artifacts && chmod -R 755 /tmp/artifacts{{end}}
{{end}}
{{if .User}}USER {{.User}}{{end}}
{{range $key, $value := .Environment}}ENV {{$key}} {{$value}}{{end}}
RUN /usr/bin/prepare
CMD [ "/usr/bin/run" ]
`))

func (h requestHandler) detectIncrementalBuild(tag string) (bool, error) {
	if h.verbose {
		log.Printf("Determining whether image %s is compatible with incremental build", tag)
	}

	container, err := h.containerFromImage(tag)
	if err != nil {
		return false, err
	}
	defer h.removeContainer(container.ID)

	return FileExistsInContainer(h.dockerClient, container.ID, "/usr/bin/save-artifacts"), nil
}

func (h requestHandler) build(req BuildRequest, incremental bool) (*BuildResult, error) {
	if h.verbose {
		log.Printf("Performing source build from %s\n", req.Source)
	}

	workingTmpDir := filepath.Join(req.WorkingDir, "tmp")
	err := os.Mkdir(workingTmpDir, 0700)
	if err != nil {
		return nil, err
	}

	if incremental {

		artifactTmpDir := filepath.Join(req.WorkingDir, "artifacts")
		err = os.Mkdir(artifactTmpDir, 0700)
		if err != nil {
			return nil, err
		}

		err = h.saveArtifacts(req.Tag, workingTmpDir, artifactTmpDir)
		if err != nil {
			return nil, err
		}
	}

	targetSourceDir := filepath.Join(req.WorkingDir, "src")
	err = h.prepareSourceDir(req.Source, targetSourceDir, req.Ref)
	if err != nil {
		return nil, err
	}

	return h.buildDeployableImage(req, req.BaseImage, req.WorkingDir, incremental)
}

func (h requestHandler) saveArtifacts(image string, tmpDir string, path string) error {
	if h.verbose {
		log.Printf("Saving build artifacts from image %s to path %s\n", image, path)
	}

	imageMetadata, err := h.dockerClient.InspectImage(image)
	if err != nil {
		return err
	}

	user := imageMetadata.ContainerConfig.User
	hasUser := (user != "")

	volumeMap := make(map[string]struct{})
	volumeMap["/tmp/artifacts"] = struct{}{}
	cmd := []string{"/usr/bin/save-artifacts"}

	if hasUser {
		volumeMap["/.container.init"] = struct{}{}
		cmd = []string{"/.container.init/init.sh"}
	}

	config := docker.Config{User: "root", Image: image, Cmd: cmd, Volumes: volumeMap}
	if h.verbose {
		log.Printf("Creating container using config: %+v\n", config)
	}
	container, err := h.dockerClient.CreateContainer(docker.CreateContainerOptions{Name: "", Config: &config})
	if err != nil {
		return err
	}
	defer h.removeContainer(container.ID)

	binds := []string{path + ":/tmp/artifacts"}
	if hasUser {
		// TODO: add custom errors?
		if h.verbose {
			log.Println("Creating stub file")
		}
		stubFile, err := openFileExclusive(filepath.Join(path, ".stub"), 0666)
		if err != nil {
			return err
		}
		defer stubFile.Close()

		containerInitDir := filepath.Join(tmpDir, ".container.init")
		if h.verbose {
			log.Printf("Creating dir %+v\n", containerInitDir)
		}
		err = os.MkdirAll(containerInitDir, 0700)
		if err != nil {
			return err
		}

		initScriptPath := filepath.Join(containerInitDir, "init.sh")
		if h.verbose {
			log.Printf("Writing %+v\n", initScriptPath)
		}
		initScript, err := openFileExclusive(initScriptPath, 0766)
		if err != nil {
			return err
		}

		err = saveArtifactsInitTemplate.Execute(initScript, struct{ User string }{user})
		if err != nil {
			return err
		}
		initScript.Close()

		binds = append(binds, containerInitDir+":/.container.init")
	}

	hostConfig := docker.HostConfig{Binds: binds}
	if h.verbose {
		log.Printf("Starting container with host config %+v\n", hostConfig)
	}
	err = h.dockerClient.StartContainer(container.ID, &hostConfig)
	if err != nil {
		return err
	}

	attachOpts := docker.AttachToContainerOptions{Container: container.ID, OutputStream: os.Stdout,
		ErrorStream: os.Stdout, Stream: true, Stdout: true, Stderr: true, Logs: true}
	err = h.dockerClient.AttachToContainer(attachOpts)
	if err != nil {
		log.Printf("Couldn't attach to container")
	}

	exitCode, err := h.dockerClient.WaitContainer(container.ID)
	if err != nil {
		return err
	}

	if exitCode != 0 {
		if h.verbose {
			log.Printf("Exit code: %d", exitCode)
		}
		return ErrSaveArtifactsFailed
	}

	return nil
}

func (h requestHandler) prepareSourceDir(source, targetSourceDir, ref string) error {
	// Support git:// and https:// schema for GIT repositories
	if strings.HasPrefix(source, "git://") || strings.HasPrefix(source, "https://") {
		if ref != "" {
			valid := validateGitRef(ref)
			if !valid {
				return ErrInvalidRef
			}
		}

		if h.verbose {
			log.Printf("Cloning %s to directory %s", source, targetSourceDir)
		}

		output, err := gitClone(source, targetSourceDir)
		if err != nil {
			if h.verbose {
				log.Printf("Git clone output:\n%s", output)
				log.Printf("Git clone failed: %+v", err)
			}

			return err
		}

		if ref != "" {
			if h.verbose {
				log.Printf("Checking out ref %s", ref)
			}

			err := gitCheckout(targetSourceDir, ref, h.verbose)
			if err != nil {
				return err
			}
		}
	} else {
		// TODO: investigate using bind-mounts instead
		copy(source, targetSourceDir)
	}

	return nil
}

func (h requestHandler) buildDeployableImage(req BuildRequest, image string, contextDir string, incremental bool) (*BuildResult, error) {
	if req.Method == "run" {
		return h.buildDeployableImageWithDockerRun(req, image, contextDir, incremental)
	}

	return h.buildDeployableImageWithDockerBuild(req, image, contextDir, incremental)
}

func (h requestHandler) buildDeployableImageWithDockerBuild(req BuildRequest, image string, contextDir string, incremental bool) (*BuildResult, error) {
	dockerFilePath := filepath.Join(contextDir, "Dockerfile")
	dockerFile, err := openFileExclusive(dockerFilePath, 0700)
	if err != nil {
		return nil, err
	}
	defer dockerFile.Close()

	imageMetadata, err := h.dockerClient.InspectImage(image)

	// If image does not exists locally, pull it from Docker registry and then
	// retry the build
	if err == docker.ErrNoSuchImage {
		imageMetadata, err = h.pullImage(image)
	}

	if err != nil {
		return nil, err
	}

	user := imageMetadata.ContainerConfig.User

	templateFiller := struct {
		BaseImage   string
		Environment map[string]string
		Incremental bool
		User        string
	}{image, req.Environment, incremental, user}
	err = dockerFileTemplate.Execute(dockerFile, templateFiller)
	if err != nil {
		return nil, ErrCreateDockerfileFailed
	}

	if h.verbose {
		log.Printf("Wrote Dockerfile for build to %s\n", dockerFilePath)
	}

	tarBall, err := tarDirectory(contextDir)
	if err != nil {
		return nil, err
	}

	if h.verbose {
		log.Printf("Created tarball for %s at %s\n", contextDir, tarBall.Name())
	}

	tarInput, err := os.Open(tarBall.Name())
	if err != nil {
		return nil, err
	}
	defer tarInput.Close()
	tarReader := bufio.NewReader(tarInput)
	var output []string

	if req.Writer != nil {
		err = h.dockerClient.BuildImage(docker.BuildImageOptions{req.Tag, false, false, true, tarReader, req.Writer, ""})
	} else {
		var buf []byte
		writer := bytes.NewBuffer(buf)
		err = h.dockerClient.BuildImage(docker.BuildImageOptions{req.Tag, false, false, true, tarReader, writer, ""})
		rawOutput := writer.String()
		output = strings.Split(rawOutput, "\n")
	}

	if err != nil {
		return nil, err
	}

	return &BuildResult{true, output}, nil
}

func (h requestHandler) buildDeployableImageWithDockerRun(req BuildRequest, image string, contextDir string, incremental bool) (*BuildResult, error) {
	volumeMap := make(map[string]struct{})
	volumeMap["/tmp/src"] = struct{}{}
	if incremental {
		volumeMap["/tmp/artifacts"] = struct{}{}
	}

	imageMetadata, err := h.dockerClient.InspectImage(image)

	if err == docker.ErrNoSuchImage {
		imageMetadata, err = h.pullImage(image)
	}

	if err != nil {
		return nil, err
	}

	user := imageMetadata.ContainerConfig.User
	hasUser := (user != "")

	cmd := []string{"/usr/bin/prepare"}
	if hasUser {
		cmd = []string{"/.container.init/init.sh"}
		volumeMap["/.container.init"] = struct{}{}
	}

	config := docker.Config{User: "root", Image: image, Cmd: cmd, Volumes: volumeMap}
	var cmdEnv []string
	if len(req.Environment) > 0 {
		for key, val := range req.Environment {
			cmdEnv = append(cmdEnv, key+"="+val)
		}
		config.Env = cmdEnv
	}
	if h.verbose {
		log.Printf("Creating container using config: %+v\n", config)
	}

	container, err := h.dockerClient.CreateContainer(docker.CreateContainerOptions{Name: "", Config: &config})
	if err != nil {
		return nil, err
	}
	defer h.removeContainer(container.ID)

	binds := []string{
		filepath.Join(contextDir, "src") + ":/tmp/src",
	}
	if incremental {
		binds = append(binds, filepath.Join(contextDir, "artifacts")+":/tmp/artifacts")
	}
	if hasUser {
		containerInitDir := filepath.Join(req.WorkingDir, "tmp", ".container.init")
		err := os.MkdirAll(containerInitDir, 0700)
		if err != nil {
			return nil, err
		}

		buildScriptPath := filepath.Join(containerInitDir, "init.sh")
		buildScript, err := openFileExclusive(buildScriptPath, 0700)
		if err != nil {
			return nil, err
		}

		templateFiller := struct {
			User        string
			Incremental bool
		}{user, incremental}

		err = buildTemplate.Execute(buildScript, templateFiller)
		if err != nil {
			return nil, err
		}
		buildScript.Close()

		binds = append(binds, containerInitDir+":/.container.init")
	}

	hostConfig := docker.HostConfig{Binds: binds}
	if h.verbose {
		log.Printf("Starting container with config: %+v\n", hostConfig)
	}

	err = h.dockerClient.StartContainer(container.ID, &hostConfig)
	if err != nil {
		return nil, err
	}

	attachOpts := docker.AttachToContainerOptions{Container: container.ID, OutputStream: os.Stdout,
		ErrorStream: os.Stdout, Stream: true, Stdout: true, Stderr: true, Logs: true}
	err = h.dockerClient.AttachToContainer(attachOpts)
	if err != nil {
		log.Printf("Couldn't attach to container")
	}

	exitCode, err := h.dockerClient.WaitContainer(container.ID)
	if err != nil {
		return nil, err
	}

	if exitCode != 0 {
		return nil, ErrBuildFailed
	}

	config = docker.Config{Image: image, Cmd: []string{"/usr/bin/run"}, Env: cmdEnv}
	if hasUser {
		config.User = user
	}

	if h.verbose {
		log.Printf("Commiting container with config: %+v\n", config)
	}

	builtImage, err := h.dockerClient.CommitContainer(docker.CommitContainerOptions{Container: container.ID, Repository: req.Tag, Run: &config})
	if err != nil {
		return nil, ErrBuildFailed
	}

	if h.verbose {
		log.Printf("Built image: %+v\n", builtImage)
	}

	return &BuildResult{true, nil}, nil
}

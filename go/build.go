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
	Writer      io.Writer
	CallbackUrl string
	ScriptsUrl  string
}

type BuildResult STIResult

// Build processes a BuildRequest and returns a *BuildResult and an error.
// An error represents a failure performing the build rather than a failure
// of the build itself.  Callers should check the Success field of the result
// to determine whether a build succeeded or not.
func Build(req BuildRequest) (*BuildResult, error) {
	h, err := newHandler(req.Request)
	if err != nil {
		return nil, err
	}

	var result *BuildResult

	result, err = h.build(req)

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
var saveArtifactsInitTemplate = template.Must(template.New("sa-init.sh").Parse(`#!/bin/sh
chown -R {{.User}}:{{.User}} /tmp/artifacts && chmod -R 755 /tmp/artifacts
chown -R {{.User}}:{{.User}} /tmp/scripts && chmod -R 755 /tmp/scripts
chown -R {{.User}}:{{.User}} /tmp/defaultScripts && chmod -R 755 /tmp/defaultScripts
chown -R {{.User}}:{{.User}} /tmp/src && chmod -R 755 /tmp/src
exec su {{.User}} -s /bin/sh -c {{.SaveArtifactsPath}}
`))

// Script used to initialize permissions on bind-mounts for a docker-run build (prepare call)
var buildTemplate = template.Must(template.New("build-init.sh").Parse(`#!/bin/sh
chown -R {{.User}}:{{.User}} /tmp/src && chmod -R 755 /tmp/src
chown -R {{.User}}:{{.User}} /tmp/scripts && chmod -R 755 /tmp/scripts
chown -R {{.User}}:{{.User}} /tmp/defaultScripts && chmod -R 755 /tmp/defaultScripts
{{if .Incremental}}chown -R {{.User}}:{{.User}} /tmp/artifacts && chmod -R 755 /tmp/artifacts{{end}}
mkdir -p /opt/sti/bin
if [ -f {{.RunPath}} ]; then
	cp {{.RunPath}} /opt/sti/bin
fi

if [ -f {{.AssemblePath}} ]; then
	exec su {{.User}} -s /bin/sh -c {{.AssemblePath}}
else 
  echo "No assemble script supplied in ScriptsUrl argument, application source, or default url in the image."
fi
	
`))

func (h requestHandler) build(req BuildRequest) (*BuildResult, error) {

	workingTmpDir := filepath.Join(req.WorkingDir, "tmp")
	dirs := []string{"tmp", "scripts", "defaultScripts"}
	for _, v := range dirs {
		err := os.Mkdir(filepath.Join(req.WorkingDir, v), 0700)
		if err != nil {
			return nil, err
		}
	}

	if req.ScriptsUrl != "" {
		h.downloadScripts(req.ScriptsUrl, filepath.Join(req.WorkingDir, "scripts"))
	}

	defaultUrl, err := h.getDefaultUrl(req, req.BaseImage)
	if err != nil {
		return nil, err
	}
	if defaultUrl != "" {
		h.downloadScripts(defaultUrl, filepath.Join(req.WorkingDir, "defaultScripts"))
	}

	targetSourceDir := filepath.Join(req.WorkingDir, "src")
	err = h.prepareSourceDir(req.Source, targetSourceDir, req.Ref)
	if err != nil {
		return nil, err
	}

	incremental := !req.Clean

	if incremental {
		// can only do incremental build if runtime image exists
		var err error
		incremental, err = h.isImageInLocalRegistry(req.Tag)
		if err != nil {
			return nil, err
		}
	}
	if incremental {
		// check if a save-artifacts script exists in anything provided to the build
		// without it, we cannot do incremental builds
		incremental = h.determineScriptPath(req.WorkingDir, "save-artifacts") != ""
	}

	if incremental {
		log.Printf("Existing image for tag %s detected for incremental build.\n", req.Tag)
	} else {
		log.Println("Clean build will be performed")
	}

	if h.verbose {
		log.Printf("Performing source build from %s\n", req.Source)
	}

	if incremental {
		artifactTmpDir := filepath.Join(req.WorkingDir, "artifacts")
		err = os.Mkdir(artifactTmpDir, 0700)
		if err != nil {
			return nil, err
		}

		err = h.saveArtifacts(req, req.Tag, workingTmpDir, artifactTmpDir, req.WorkingDir)
		if err != nil {
			return nil, err
		}
	}

	return h.buildDeployableImage(req, req.BaseImage, req.WorkingDir, incremental)
}

func (h requestHandler) getDefaultUrl(req BuildRequest, image string) (string, error) {
	imageMetadata, err := h.dockerClient.InspectImage(image)
	if err != nil {
		return "", err
	}
	var defaultScriptsUrl string
	env := imageMetadata.ContainerConfig.Env
	for _, v := range env {
		if strings.HasPrefix(v, "STI_SCRIPTS_URL=") {
			t := strings.Split(v, "=")
			defaultScriptsUrl = t[1]
			break
		}
	}
	if h.verbose {
		log.Printf("Image contains default script url %s", defaultScriptsUrl)
	}
	return defaultScriptsUrl, nil
}

func (h requestHandler) determineScriptPath(contextDir string, script string) string {
	if _, err := os.Stat(filepath.Join(contextDir, "scripts", script)); err == nil {
		// if the invoker provided a script via a url, prefer that.
		if h.verbose {
			log.Printf("Using %s script from user provided url", script)
		}
		return filepath.Join("/tmp", "scripts", script)
	} else if _, err := os.Stat(filepath.Join(contextDir, "src", ".sti", "bin", script)); err == nil {
		// if they provided one in the app source, that is preferred next
		if h.verbose {
			log.Printf("Using %s script from application source", script)
		}
		return filepath.Join("/tmp", "src", ".sti", "bin", script)
	} else if _, err := os.Stat(filepath.Join(contextDir, "defaultScripts", script)); err == nil {
		// lowest priority: script provided by default url reference in the image.
		if h.verbose {
			log.Printf("Using %s script from image default url", script)
		}
		return filepath.Join("/tmp", "defaultScripts", script)
	}
	return ""
}

func (h requestHandler) downloadScripts(url, targetDir string) error {
	os.MkdirAll(targetDir, 0700)
	files := []string{"save-artifacts", "assemble", "run"}
	for _, file := range files {
		if h.verbose {
			log.Printf("Downloading file %s from url %s to directory %s\n", file, url, targetDir)
		}

		resp, err := http.Get(url + "/" + file)
		defer resp.Body.Close()
		if err != nil {
			return err
		}
		if resp.StatusCode == 200 || resp.StatusCode == 201 {
			out, err := os.Create(targetDir + "/" + file)
			defer out.Close()
			if err != nil {
				return err
			}
			_, err = io.Copy(out, resp.Body)
		} else {
			//return fmt.Errorf("Failed to retrieve %s from %s, response code %d", targetFile, targetDir, resp.StatusCode)
			log.Printf("Failed to retrieve %s from %s, response code %d", file, targetDir, resp.StatusCode)
		}
	}
	return nil
}

func (h requestHandler) saveArtifacts(req BuildRequest, image string, tmpDir string, path string, contextDir string) error {
	if h.verbose {
		log.Printf("Saving build artifacts from image %s to path %s\n", image, path)
	}

	imageMetadata, err := h.dockerClient.InspectImage(image)
	if err != nil {
		return err
	}
	saveArtifactsScriptPath := h.determineScriptPath(req.WorkingDir, "save-artifacts")
	user := imageMetadata.Config.User
	hasUser := (user != "")
	log.Printf("Artifact image hasUser=%t, user is %s", hasUser, user)
	volumeMap := make(map[string]struct{})
	volumeMap["/tmp/artifacts"] = struct{}{}
	volumeMap["/tmp/src"] = struct{}{}
	volumeMap["/tmp/scripts"] = struct{}{}
	volumeMap["/tmp/defaultScripts"] = struct{}{}
	cmd := []string{"/bin/sh", "-c", "chmod 777 " + saveArtifactsScriptPath + " && " + saveArtifactsScriptPath}
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
	binds = append(binds, filepath.Join(contextDir, "src")+":/tmp/src")
	binds = append(binds, filepath.Join(contextDir, "defaultScripts")+":/tmp/defaultScripts")
	binds = append(binds, filepath.Join(contextDir, "scripts")+":/tmp/scripts")

	if hasUser {
		// TODO: add custom errors?
		if h.verbose {
			log.Println("Creating stub file")
		}
		stubFile, err := os.OpenFile(filepath.Join(path, ".stub"), os.O_CREATE|os.O_RDWR, 0666)
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
		initScript, err := os.OpenFile(initScriptPath, os.O_CREATE|os.O_RDWR, 0766)
		if err != nil {
			return err
		}

		err = saveArtifactsInitTemplate.Execute(initScript, struct {
			User              string
			SaveArtifactsPath string
		}{user, saveArtifactsScriptPath})
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
	if validCloneSpec(source, h.verbose) {
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
	log.Printf("Building with docker run invocation\n")
	volumeMap := make(map[string]struct{})
	volumeMap["/tmp/src"] = struct{}{}
	volumeMap["/tmp/scripts"] = struct{}{}
	volumeMap["/tmp/defaultScripts"] = struct{}{}
	if incremental {
		volumeMap["/tmp/artifacts"] = struct{}{}
	}

	if h.verbose {
		log.Printf("Using image name %s", image)
	}
	imageMetadata, err := h.dockerClient.InspectImage(image)

	if err == docker.ErrNoSuchImage {
		imageMetadata, err = h.pullImage(image)
	}

	if err != nil {
		return nil, err
	}

	origCmd := imageMetadata.Config.Cmd
	origEntrypoint := imageMetadata.Config.Entrypoint

	runPath := h.determineScriptPath(req.WorkingDir, "run")
	assemblePath := h.determineScriptPath(req.WorkingDir, "assemble")
	overrideRun := runPath != ""

	log.Printf("Using run script from %s", runPath)
	log.Printf("Using assemble script from %s", assemblePath)

	user := imageMetadata.Config.User
	hasUser := (user != "")
	if hasUser {
		log.Printf("Image has username %s", user)
	}

	if assemblePath == "" {
		return nil, fmt.Errorf("No assemble script found in provided url, application source, or default image url.  Aborting.")
	}

	cmd := []string{"/bin/sh", "-c", "chmod 700 " + assemblePath + " && " + assemblePath + " && mkdir -p /opt/sti/bin && cp " + runPath + " /opt/sti/bin && chmod 700 /opt/sti/bin/run"}
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
	binds = append(binds, filepath.Join(contextDir, "defaultScripts")+":/tmp/defaultScripts")
	binds = append(binds, filepath.Join(contextDir, "scripts")+":/tmp/scripts")
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
		buildScript, err := os.OpenFile(buildScriptPath, os.O_CREATE|os.O_RDWR, 0700)
		if err != nil {
			return nil, err
		}

		templateFiller := struct {
			User         string
			Incremental  bool
			AssemblePath string
			RunPath      string
		}{user, incremental, assemblePath, runPath}

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

	config = docker.Config{Image: image, Env: cmdEnv}
	if overrideRun {
		config.Cmd = []string{"/opt/sti/bin/run"}
	} else {
		config.Cmd = origCmd
		config.Entrypoint = origEntrypoint
	}
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

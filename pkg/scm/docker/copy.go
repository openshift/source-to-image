package docker

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
	"github.com/openshift/source-to-image/pkg/api"
	dockerpkg "github.com/openshift/source-to-image/pkg/docker"
	"github.com/openshift/source-to-image/pkg/errors"
	"github.com/openshift/source-to-image/pkg/scm/file"
	"github.com/openshift/source-to-image/pkg/tar"
	"github.com/openshift/source-to-image/pkg/util"
)

type Docker struct {
}

func (d *Docker) Download(config *api.Config) (*api.SourceInfo, error) {
	dockerclient, err := dockerpkg.New(config.DockerConfig, config.PullAuthentication)
	if err != nil {
		return nil, err
	}
	image, sourcePath := parseDockerSource(config.Source)
	targetDir := filepath.Join(config.WorkingDir, "upload", "sources")

	if config.ForcePull {
		if _, err = dockerclient.PullImage(image); err != nil {
			glog.Warningf("Failed to pull %q (%v), searching for the local image ...", image, err)
			_, err = dockerclient.CheckImage(image)
		}
	} else {
		_, err = dockerclient.CheckAndPullImage(image)
	}

	if err != nil {
		return nil, err
	}

	// Copy the files from the container
	outReader, outWriter := io.Pipe()
	errReader, errWriter := io.Pipe()
	defer errReader.Close()
	defer errWriter.Close()
	glog.V(1).Infof("Downloading sources from %q to %q", image, targetDir)
	extractFunc := func() error {
		defer outReader.Close()
		return tar.New().ExtractTarStream(targetDir, outReader)
	}
	opts := dockerpkg.RunContainerOptions{
		Image:       image,
		ScriptsURL:  config.ScriptsURL,
		Destination: config.Destination,
		Cmd:         []string{"tar", "cf", "-", sourcePath},
		Stdout:      outWriter,
		Stderr:      errWriter,
		OnStart:     extractFunc,
	}
	go dockerpkg.StreamContainerIO(errReader, nil, glog.Error)
	err = dockerclient.RunContainer(opts)
	if e, ok := err.(errors.ContainerError); ok {
		return nil, fmt.Errorf("Failed to extract sources from %q: %v\n%s\n", image, err, e.Output)
	}

	config.Source = "file://" + targetDir
	handler := &file.File{util.NewFileSystem()}
	return handler.Download(config)
}

func parseDockerSource(s string) (image string, location string) {
	s = strings.TrimPrefix(s, "docker://")
	if !strings.Contains(s, ":") {
		image = s
	} else {
		image = s[0:strings.LastIndex(s, ":")]
	}
	if strings.HasPrefix(image, "/") {
		image = ""
		return
	}
	if strings.LastIndex(s, ":")+1 >= len(s) || !strings.Contains(s, ":") {
		location = "."
		return
	}
	return image, s[strings.LastIndex(s, ":")+1:]
}

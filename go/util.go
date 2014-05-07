package sti

import (
	"archive/tar"
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fsouza/go-dockerclient"
)

// Determine whether a file exists in a container.
func FileExistsInContainer(dockerClient *docker.Client, cId string, path string) bool {
	var buf []byte
	writer := bytes.NewBuffer(buf)

	err := dockerClient.CopyFromContainer(docker.CopyFromContainerOptions{writer, cId, path})
	content := writer.String()

	return ((err == nil) && ("" != content))
}

func stringInSlice(s string, slice []string) bool {
	for _, element := range slice {
		if s == element {
			return true
		}
	}

	return false
}

func writeTar(tw *tar.Writer, path string, relative string, fi os.FileInfo) error {
	fr, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fr.Close()

	h := new(tar.Header)
	h.Name = strings.Replace(path, relative, ".", 1)
	h.Size = fi.Size()
	h.Mode = int64(fi.Mode())
	h.ModTime = fi.ModTime()

	err = tw.WriteHeader(h)
	if err != nil {
		return err
	}

	_, err = io.Copy(tw, fr)
	return err
}

func tarDirectory(dir string) (*os.File, error) {
	fw, err := ioutil.TempFile("", "sti-tar")
	if err != nil {
		return nil, err
	}
	defer fw.Close()

	tw := tar.NewWriter(fw)
	defer tw.Close()

	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			err = writeTar(tw, path, dir, info)
			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return fw, nil
}

func copy(sourcePath string, targetPath string) error {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return err
	}

	if !info.IsDir() {
		err = os.Mkdir(targetPath, 0700)
		if err != nil {
			return err
		}

		targetPath = filepath.Join(targetPath, filepath.Base(sourcePath))
	}

	cmd := exec.Command("cp", "-ad", sourcePath, targetPath)
	return cmd.Run()
}

func imageHasEntryPoint(image *docker.Image) bool {
	found := (image.ContainerConfig.Entrypoint != nil)

	if !found && image.Config != nil {
		found = image.Config.Entrypoint != nil
	}

	return found
}

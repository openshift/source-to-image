package util

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
)

type FileSystem interface {
	Chmod(file string, mode os.FileMode) error
	Rename(from, to string) error
	MkdirAll(dirname string) error
	Mkdir(dirname string) error
	Exists(file string) bool
	Copy(sourcePath, targetPath string) error
	RemoveDirectory(dir string) error
	CreateWorkingDirectory() (string, error)
	Open(file string) (io.ReadCloser, error)
}

type fs struct {
	verbose bool
	runner  CommandRunner
}

func NewFileSystem(verbose bool) FileSystem {
	return &fs{
		verbose: verbose,
		runner:  NewCommandRunner(),
	}
}

func (h *fs) Chmod(file string, mode os.FileMode) error {
	return os.Chmod(file, mode)
}

func (h *fs) Rename(from, to string) error {
	return os.Rename(from, to)
}

func (h *fs) MkdirAll(dirname string) error {
	return os.MkdirAll(dirname, 0700)
}

func (h *fs) Mkdir(dirname string) error {
	return os.Mkdir(dirname, 0700)
}

func (h *fs) Exists(file string) bool {
	_, err := os.Stat(file)
	return err == nil
}

func (h *fs) Copy(sourcePath string, targetPath string) error {
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
	return h.runner.Run("cp", "-ad", sourcePath, targetPath)
}

func (h *fs) RemoveDirectory(dir string) error {
	if h.verbose {
		log.Printf("Removing directory '%s'\n", dir)
	}

	err := os.RemoveAll(dir)
	if err != nil {
		log.Printf("Error removing directory '%s': %s\n", dir, err.Error())
	}
	return err
}

func (h *fs) CreateWorkingDirectory() (directory string, err error) {
	directory, err = ioutil.TempDir("", "sti")
	if err != nil {
		return "", fmt.Errorf("Error creating temporary directory '%s': %s\n", directory, err.Error())
	}

	return directory, err
}

func (h *fs) Open(filename string) (io.ReadCloser, error) {
	return os.Open(filename)
}

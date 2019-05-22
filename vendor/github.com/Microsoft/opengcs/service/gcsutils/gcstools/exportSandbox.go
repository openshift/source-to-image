package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Microsoft/opengcs/service/gcsutils/gcstools/commoncli"
	"github.com/Microsoft/opengcs/service/gcsutils/libvhd2tar"
	"github.com/sirupsen/logrus"
)

func exportSandbox() error {
	vhd2tarArgs := commoncli.SetFlagsForVHD2TarLib()
	logArgs := commoncli.SetFlagsForLogging()
	mntPath := flag.String("path", "", "path to layer")
	flag.Parse()

	if err := commoncli.SetupLogging(logArgs...); err != nil {
		return err
	}

	options, err := commoncli.SetupVHD2TarLibOptions(vhd2tarArgs...)
	if err != nil {
		logrus.Infof("error: %s. Please use -h for params", err)
		return err
	}

	if *mntPath == "" {
		err = fmt.Errorf("path is required")
		logrus.Infof("error: %s. Please use -h for params", err)
		return err
	}

	absPath, err := filepath.Abs(*mntPath)
	if err != nil {
		logrus.Infof("error: %s. Could not get abs", err)
		return err
	}

	logrus.Infof("converted: Packing %s", absPath)
	if _, err = libvhd2tar.VHDX2Tar(absPath, os.Stdout, options); err != nil {
		logrus.Infof("failed to pack files: %s", err)
		return err
	}
	return nil
}

func exportSandboxMain() {
	if err := exportSandbox(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

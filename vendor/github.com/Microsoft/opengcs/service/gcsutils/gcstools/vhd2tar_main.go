package main

import (
	"flag"
	"os"

	"github.com/Microsoft/opengcs/service/gcsutils/gcstools/commoncli"
	"github.com/Microsoft/opengcs/service/gcsutils/libvhd2tar"
	"github.com/sirupsen/logrus"
)

func vhd2tar() error {
	vhd2tarArgs := commoncli.SetFlagsForVHD2TarLib()
	logArgs := commoncli.SetFlagsForLogging()
	flag.Parse()

	options, err := commoncli.SetupVHD2TarLibOptions(vhd2tarArgs...)
	if err != nil {
		logrus.Infof("error: %s. Please use -h for params", err)
		return err
	}

	if err = commoncli.SetupLogging(logArgs...); err != nil {
		logrus.Infof("error: %s. Please use -h for params", err)
		return err
	}

	if _, err = libvhd2tar.VHD2Tar(os.Stdin, os.Stdout, options); err != nil {
		logrus.Infof("svmutilsMain failed with %s", err)
		return err
	}
	return nil
}

func vhd2tarMain() {
	if err := vhd2tar(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

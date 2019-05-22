package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Microsoft/opengcs/service/gcsutils/gcstools/commoncli"
	"github.com/Microsoft/opengcs/service/gcsutils/remotefs"
	"github.com/sirupsen/logrus"
)

func remotefsHandler() error {
	logArgs := commoncli.SetFlagsForLogging()
	flag.Parse()

	if err := commoncli.SetupLogging(logArgs...); err != nil {
		logrus.Infof("error: %s. Use --help for supported flags", err)
		return err
	}

	if len(flag.Args()) < 2 {
		return remotefs.ErrUnknown
	}

	command := flag.Args()[0]
	if cmd, ok := remotefs.Commands[command]; ok {
		cmdErr := cmd(os.Stdin, os.Stdout, flag.Args()[1:])

		// Write the cmdErr to stderr, so that the client can handle it.
		if err := remotefs.WriteError(cmdErr, os.Stderr); err != nil {
			logrus.Errorf("failed to send error %v to stderr: %v", cmdErr, err)
			return err
		}
		logrus.Infof("sent '%v' back to stderr", cmdErr)
		return nil
	}

	fmt.Fprintf(os.Stderr, "unknown command: %s\n", command)
	fmt.Fprintf(os.Stderr, "known commands:\n")
	for k := range remotefs.Commands {
		fmt.Fprintf(os.Stderr, "\t%s\n", k)
	}
	return remotefs.ErrUnknown
}

func remotefsMain() {
	if err := remotefsHandler(); err != nil {
		logrus.Errorf("error in remotefsHandler: %v", err)
		os.Exit(1)
	}
	os.Exit(0)
}

package commoncli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/pkg/archive"
	"github.com/sirupsen/logrus"

	"github.com/Microsoft/opengcs/service/gcsutils/libvhd2tar"
)

// SetFlagsForVHD2TarLib creates the command line flags for the vhd2tar
// functions.
func SetFlagsForVHD2TarLib() []*string {
	filesystem := flag.String("fs", "ext4", "Filesystem format: ext4")
	whiteout := flag.String("whiteout", "overlay", "Whiteout format: aufs, overlay")
	vhdFormat := flag.String("vhd", "fixed", "VHD format: fixed")
	tempDirectory := flag.String("tmpdir", "/tmp/scratch", "Temp directory for intermediate files.")
	return []*string{filesystem, whiteout, vhdFormat, tempDirectory}
}

// SetupVHD2TarLibOptions converts the command line flags to libvhd2tar.Options.
func SetupVHD2TarLibOptions(args ...*string) (*libvhd2tar.Options, error) {
	if len(args) < 4 {
		return nil, fmt.Errorf("Mistmatched arguments for vhd2tar")
	}

	fsys := *args[0]
	wh := *args[1]
	vhdFormat := *args[2]
	tmpdir := *args[3]

	var format archive.WhiteoutFormat
	if fsys != "ext4" {
		return nil, fmt.Errorf("Unknown filesystem: %s", fsys)
	}

	if wh == "overlay" {
		format = archive.OverlayWhiteoutFormat
	} else if wh == "aufs" {
		format = archive.AUFSWhiteoutFormat
	} else {
		return nil, fmt.Errorf("Unknown whiteout format: %s", wh)
	}

	if vhdFormat != "fixed" {
		return nil, fmt.Errorf("Unknown vhd format: %s", vhdFormat)
	}

	// Note due to the semantics of MkdirAll, err == nil if the directory exists.
	if err := os.MkdirAll(tmpdir, 0755); err != nil {
		return nil, err
	}

	options := &libvhd2tar.Options{
		TarOpts: &archive.TarOptions{
			WhiteoutFormat:  format,
			ExcludePatterns: []string{`lost\+found`},
		},
		TempDirectory: tmpdir,
	}

	return options, nil
}

// SetFlagsForLogging sets the command line flags for logging.
func SetFlagsForLogging() []*string {
	basename := filepath.Base(os.Args[0]) + ".log"
	logFile := flag.String("logfile", filepath.Join("/tmp", basename), "logging file location")
	logLevel := flag.String("loglevel", "debug", "Logging Level: debug, info, warning, error, fatal, panic.")
	return []*string{logFile, logLevel}
}

// SetupLogging creates the logger from the command line parameters.
func SetupLogging(args ...*string) error {
	if len(args) < 1 {
		return fmt.Errorf("Invalid log params")
	}
	level, err := logrus.ParseLevel(*args[1])
	if err != nil {
		logrus.Fatal(err)
		return err
	}
	logrus.SetLevel(level)

	// Add the sub-command to the filename for remotefs. This is really ugly :(
	// Done to allow mixing both fixed args that remotefs expects for back-compat,
	// along with flags. Prior to this change, remotefs didn't accept flags at all,
	// and did no logging. Unfortunately, there are circumstances where two distinct
	// remotefs commands can execute simultaneously, so we need separate logfiles.
	filename := *args[0]
	if os.Args[0] == "remotefs" {
		if len(flag.Args()[0]) > 1 {
			filename = strings.Replace(filename, "remotefs", fmt.Sprintf("remotefs.%s", flag.Args()[0]), 1)
		}
	}

	outputTarget, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}

	logrus.SetOutput(outputTarget)
	return nil
}

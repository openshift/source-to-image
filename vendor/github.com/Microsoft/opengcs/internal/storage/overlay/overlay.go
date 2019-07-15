// +build linux

package overlay

import (
	"context"
	"os"
	"strings"

	"github.com/Microsoft/opengcs/internal/oc"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"golang.org/x/sys/unix"
)

// Test dependencies
var (
	osMkdirAll  = os.MkdirAll
	osRemoveAll = os.RemoveAll
	unixMount   = unix.Mount
)

// Mount creates an overlay mount with `layerPaths` at `rootfsPath`.
//
// If `upperdirPath != ""` the path will be created. On mount failure the
// created `upperdirPath` will be automatically cleaned up.
//
// If `workdirPath != ""` the path will be created. On mount failure the created
// `workdirPath` will be automatically cleaned up.
//
// Always creates `rootfsPath`. On mount failure the created `rootfsPath` will
// be automatically cleaned up.
func Mount(ctx context.Context, layerPaths []string, upperdirPath, workdirPath, rootfsPath string, readonly bool) (err error) {
	_, span := trace.StartSpan(ctx, "overlay::Mount")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	lowerdir := strings.Join(layerPaths, ":")
	span.AddAttributes(
		trace.StringAttribute("layerPaths", lowerdir),
		trace.StringAttribute("upperdirPath", upperdirPath),
		trace.StringAttribute("workdirPath", workdirPath),
		trace.StringAttribute("rootfsPath", rootfsPath),
		trace.BoolAttribute("readonly", readonly))

	if rootfsPath == "" {
		return errors.New("cannot have empty rootfsPath")
	}

	if readonly && (upperdirPath != "" || workdirPath != "") {
		return errors.Errorf("upperdirPath: %q, and workdirPath: %q must be emty when readonly==true", upperdirPath, workdirPath)
	}

	options := []string{"lowerdir=" + lowerdir}
	if upperdirPath != "" {
		if err := osMkdirAll(upperdirPath, 0755); err != nil {
			return errors.Wrap(err, "failed to create upper directory in scratch space")
		}
		defer func() {
			if err != nil {
				osRemoveAll(upperdirPath)
			}
		}()
		options = append(options, "upperdir="+upperdirPath)
	}
	if workdirPath != "" {
		if err := osMkdirAll(workdirPath, 0755); err != nil {
			return errors.Wrap(err, "failed to create workdir in scratch space")
		}
		defer func() {
			if err != nil {
				osRemoveAll(workdirPath)
			}
		}()
		options = append(options, "workdir="+workdirPath)
	}
	if err := osMkdirAll(rootfsPath, 0755); err != nil {
		return errors.Wrapf(err, "failed to create directory for container root filesystem %s", rootfsPath)
	}
	defer func() {
		if err != nil {
			osRemoveAll(rootfsPath)
		}
	}()
	var flags uintptr
	if readonly {
		flags |= unix.MS_RDONLY
	}
	if err := unixMount("overlay", rootfsPath, "overlay", flags, strings.Join(options, ",")); err != nil {
		return errors.Wrapf(err, "failed to mount container root filesystem using overlayfs %s", rootfsPath)
	}
	return nil
}

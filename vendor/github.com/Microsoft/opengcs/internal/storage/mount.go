// +build linux

package storage

import (
	"context"
	"os"

	"github.com/Microsoft/opengcs/internal/oc"
	"github.com/pkg/errors"
	"go.opencensus.io/trace"
	"golang.org/x/sys/unix"
)

// Test dependencies
var (
	osStat      = os.Stat
	unixUnmount = unix.Unmount
	osRemoveAll = os.RemoveAll
)

// UnmountPath unmounts the target path if it exists and is a mount path. If
// removeTarget this will remove the previously mounted folder.
func UnmountPath(ctx context.Context, target string, removeTarget bool) (err error) {
	_, span := trace.StartSpan(ctx, "storage::UnmountPath")
	defer span.End()
	defer func() { oc.SetSpanStatus(span, err) }()

	span.AddAttributes(
		trace.StringAttribute("target", target),
		trace.BoolAttribute("remove", removeTarget))

	if _, err := osStat(target); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to determine if path '%s' exists", target)
	}

	if err := unixUnmount(target, 0); err != nil {
		// If `Unmount` returns `EINVAL` it's not mounted. Just delete the
		// folder.
		if err != unix.EINVAL {
			return errors.Wrapf(err, "failed to unmount path '%s'", target)
		}
	}
	if removeTarget {
		return osRemoveAll(target)
	}
	return nil
}

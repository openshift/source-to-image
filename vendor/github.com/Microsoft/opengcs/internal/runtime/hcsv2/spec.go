package hcsv2

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/opencontainers/runc/libcontainer/user"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

// getNetworkNamespaceID returns the `ToLower` of
// `spec.Windows.Network.NetworkNamespace` or `""`.
func getNetworkNamespaceID(spec *oci.Spec) string {
	if spec.Windows != nil &&
		spec.Windows.Network != nil {
		return strings.ToLower(spec.Windows.Network.NetworkNamespace)
	}
	return ""
}

// isRootReadonly returns `true` if the spec specifies the rootfs is readonly.
func isRootReadonly(spec *oci.Spec) bool {
	if spec.Root != nil {
		return spec.Root.Readonly
	}
	return false
}

// isInMounts returns `true` if `target` matches a `Destination` in any of
// `mounts`.
func isInMounts(target string, mounts []oci.Mount) bool {
	for _, m := range mounts {
		if m.Destination == target {
			return true
		}
	}
	return false
}

func setProcess(spec *oci.Spec) {
	if spec.Process == nil {
		spec.Process = &oci.Process{}
	}
}

// setUserStr sets `spec.Process` to the valid `userstr` based on the OCI Image
// Spec v1.0.0 `userstr`.
//
// Valid values are: user, uid, user:group, uid:gid, uid:group, user:gid
func setUserStr(spec *oci.Spec, userstr string) error {
	setProcess(spec)

	parts := strings.Split(userstr, ":")
	switch len(parts) {
	case 1:
		v, err := strconv.Atoi(parts[0])
		if err != nil {
			// evaluate username to uid/gid
			return setUsername(spec, userstr)
		}
		return setUserID(spec, int(v))
	case 2:
		var (
			username, groupname string
			uid, gid            int
		)
		v, err := strconv.Atoi(parts[0])
		if err != nil {
			username = parts[0]
		} else {
			uid = int(v)
		}
		v, err = strconv.Atoi(parts[1])
		if err != nil {
			groupname = parts[1]
		} else {
			gid = int(v)
		}
		if username != "" {
			u, err := getUser(spec, func(u user.User) bool {
				return u.Name == username
			})
			if err != nil {
				return errors.Wrapf(err, "failed to find user by username: %s", username)
			}
			uid = u.Uid
		}
		if groupname != "" {
			g, err := getGroup(spec, func(g user.Group) bool {
				return g.Name == groupname
			})
			if err != nil {
				return errors.Wrapf(err, "failed to find group by groupname: %s", groupname)
			}
			gid = g.Gid
		}
		spec.Process.User.UID, spec.Process.User.GID = uint32(uid), uint32(gid)
		return nil
	default:
		return fmt.Errorf("invalid userstr: '%s'", userstr)
	}
}

func setUsername(spec *oci.Spec, username string) error {
	u, err := getUser(spec, func(u user.User) bool {
		return u.Name == username
	})
	if err != nil {
		return errors.Wrapf(err, "failed to find user by username: %s", username)
	}
	spec.Process.User.UID, spec.Process.User.GID = uint32(u.Uid), uint32(u.Gid)
	return nil
}

func setUserID(spec *oci.Spec, uid int) error {
	u, err := getUser(spec, func(u user.User) bool {
		return u.Uid == uid
	})
	if err != nil {
		return errors.Wrapf(err, "failed to find user by uid: %d", uid)
	}
	spec.Process.User.UID, spec.Process.User.GID = uint32(u.Uid), uint32(u.Gid)
	return nil
}

func getUser(spec *oci.Spec, filter func(user.User) bool) (user.User, error) {
	users, err := user.ParsePasswdFileFilter(filepath.Join(spec.Root.Path, "/etc/passwd"), filter)
	if err != nil {
		return user.User{}, err
	}
	if len(users) != 1 {
		return user.User{}, errors.Errorf("expected exactly 1 user matched '%d'", len(users))
	}
	return users[0], nil
}

func getGroup(spec *oci.Spec, filter func(user.Group) bool) (user.Group, error) {
	groups, err := user.ParseGroupFileFilter(filepath.Join(spec.Root.Path, "/etc/group"), filter)
	if err != nil {
		return user.Group{}, err
	}
	if len(groups) != 1 {
		return user.Group{}, errors.Errorf("expected exactly 1 group matched '%d'", len(groups))
	}
	return groups[0], nil
}

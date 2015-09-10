package scm

import (
	"strings"

	"github.com/golang/glog"
	"github.com/openshift/source-to-image/pkg/build"
	"github.com/openshift/source-to-image/pkg/scm/file"
	"github.com/openshift/source-to-image/pkg/scm/git"
	"github.com/openshift/source-to-image/pkg/util"
)

func DownloaderForSource(s string) build.Downloader {
	if strings.HasPrefix(s, "file://") || strings.HasPrefix(s, "/") {
		return &file.File{util.NewFileSystem()}
	}
	g := git.New()
	if g.ValidCloneSpec(s) {
		return &git.Clone{g, util.NewFileSystem()}
	}

	glog.Errorf("No downloader defined for %q source URL", s)
	return nil
}

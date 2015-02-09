package strategies

import (
	"strings"

	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/build"
	"github.com/openshift/source-to-image/pkg/build/strategies/onbuild"
	"github.com/openshift/source-to-image/pkg/build/strategies/sti"
)

func GetBuilder(request *api.Request) (build.Builder, error) {
	// TODO: The ONBUILD instructions should be available in ImageInspect call,
	// however they are not in the current go-dockerclient version.
	// Right now we assume that if the base image has '-onbuild' in the tag, then
	// we do OnBuild build.
	if strings.Contains(request.BaseImage, "-onbuild") {
		return onbuild.NewOnBuild(request)
	} else {
		return sti.NewSTI(request)
	}
}

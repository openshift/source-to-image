go-sti
======

This is the golang implementation of the source-to-images tool.

### Getting started

#### Dependencies

1. [Docker](http://www.docker.io)
1. [Go](http://golang.org/)

#### Installation

	go get github.com/openshift/docker-source-to-images/go

#### Example

You can start using sti right away with the following test sources and publicly available images:

    sti build git://github.com/pmorie/simple-ruby pmorie/centos-ruby2 test-ruby-app
    docker run -rm -i -p :9292 -t test-ruby-app

    sti build git://github.com/pmorie/simple-ruby pmorie/ubuntu-buildpack test-foreman-app \
    -e 'BUILDPACK_URL=https://github.com/heroku/heroku-buildpack-ruby.git'
    docker run -rm -i -p :5000 -t test-foreman-app

    sti build git://github.com/pmorie/simple-html pmorie/fedora-mock test-html-app
    docker run -rm -i -p :8080 -t sti_app


### Validating a source image

    sti validate BUILD_IMAGE_TAG [flags]

    Available Flags:
         --debug=false: Enable debugging output
     -I, --incremental=false: Validate for an incremental build
     -R, --runtime="": Set the runtime image to use
     -U, --url="unix:///var/run/docker.sock": Set the url of the docker socket to use


You can validate that an image is usable as a sti source image as follows:

	sti validate BUILD_IMAGE_TAG

The `--incremental` option to enables validation for incremental builds:

    sti validate BUILD_IMAGE_TAG --incremental

Add the `-R` option to additionally validate a runtime image for extended builds:

    sti validate BUILD_IMAGE_TAG -R RUNTIME_IMAGE_TAG

When specifying a runtime image with `sti validate`, the build image is automatically validated for
incremental builds.

### Building a deployable image with sti

    sti build SOURCE BUILD_IMAGE APP_IMAGE_TAG [flags]

    Available Flags:
         --clean=false: Perform a clean build
         --debug=false: Enable debugging output
         --dir="tempdir": Directory where generated Dockerfiles and other support scripts are created
     -e, --env="": Specify an environment var NAME=VALUE,NAME2=VALUE2,...
     -R, --runtime="": Set the runtime image to use
     -U, --url="unix:///var/run/docker.sock": Set the url of the docker socket to use


The most basic `sti build` uses a single build image:

    sti build SOURCE BUILD_IMAGE_TAG APP_IMAGE_TAG

If the build is successful, the built image will be tagged with `APP_IMAGE_TAG`.

If the build image is compatible with incremental builds, `sti build` will look for an image tagged
with `APP_IMAGE_TAG`.  If an image is present with that tag, `sti build` will save the build
artifacts from that image and add them to the build container at `/tmp/artifacts` so an image's
`/usr/bin/prepare` script can restore them before building the source.

When using an image that supports incremental builds, you can do a clean build with `--clean`:

    sti build SOURCE BUILD_IMAGE_TAG APP_IMAGE_TAG --clean

Extended builds allow you to use distinct images for building your sources and deploying them. Use
the `-R` option perform an extended build targeting a runtime image:

    sti build SOURCE BUILD_IMAGE_TAG APP_IMAGE_TAG -R RUNTIME_IMAGE_TAG

When specifying a runtime image, the build image must be compatible with incremental builds.
`sti build` will look for an image tagged with `<APP_IMAGE_TAG>-build`.  If an image is present with
that tag, `sti build` will save the build artifacts from that image and add them to the build
container at `/tmp/artifacts` so the build image's `/usr/bin/prepare` script can restore them before
building the source.  The build image's `/usr/bin/prepare` script is responsible for populating
`/tmp/build` with an artifact to be deployed into the runtime container.

After performing the build, a new runtime image is created based on the image tagged with
`RUNTIME_IMAGE_TAG` with the output of the build in `/tmp/src`.  The runtime image's
`/usr/bin/prepare` script is responsible for detecting and deploying the artifact.  If the build is
successful, two images are tagged:

1. The build image is tagged with `<APP_IMAGE_TAG>-build`
1. The prepared image incorporating the deployed build is tagged with `APP_IMAGE_TAG`

You can do a clean extended build with `--clean`:

    sti build SOURCE_DIR BUILD_IMAGE_TAG APP_IMAGE_TAG -R RUNTIME_IMAGE_TAG --clean

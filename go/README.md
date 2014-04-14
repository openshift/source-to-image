go-sti
======

Source-to-images (`sti`) is a tool for building reproducable Docker images.  `sti`
produces ready-to-run images by injecting a user source into a docker image and <i>preparing</i>
a new Docker image which incorporates the base image and built source, and is ready to use
with `docker run`.  `sti` supports:

1. Incremental builds which re-use previously downloaded dependencies, previously built
   artifacts, etc
1. Build on one image, deploy on another with extended builds

Interested in learning more?  Read on!

### Philosophy

1. Simplify the process of application source + base image -> usable image for most use cases (the 80%)
2. Define and implement a workflow for incremental build that eventually uses only docker primitives
3. Develop tooling that can assist in verifying that two different base images result in the same
   "docker run" outcome for the same input
4. Use native docker primitives to accomplish this - map out useful improvements to docker that
   benefit all image builders

### Anatomy of a source image

Building source images is as easy as implementing two scripts.  `sti` expects the
following scripts in `/usr/bin`:

1. `prepare` : This script is responsible for building and/or deploying the source
1. `run`: This script is responsible for running the deployed source

### Build methodologies

`sti` implements two methodologies for building Docker images.  The first will be familiar to anyone
who's built their own Docker image before - it's just `docker build`.  When building this way, `sti`
generates a Dockerfile and calls `docker build` to produce the output image:

1. `sti` generates a `Dockerfile` to describe the output image:
    1. Based on the build image
    1. Adds the application source at `/tmp/src` in the container
    1. Calls `/usr/bin/prepare` in the container
    1. Sets the image's default command to `/usr/bin/run`
1. `sti` calls `docker build` to produce the output image

`sti` also supports building images with `docker run`.  When building this way, the workflow is:

1. `sti` creates a container based on the build image. with:
    1. The application source bind-mounted to `/tmp/src`
    1. The build artifacts bind-mounted to `/tmp/artifacts` (if applicable - see incremental builds)
    1. Runs the build image's `/usr/bin/prepare` script
1. `sti` starts the container and waits for it to finish running
1. `sti` commits the container, setting the CMD for the output image to be `/usr/bin/run`

The build methodology is controlled by the `-m` option, and defaults to `build`.  To build with
`docker run`, use `-m run`.

### Basic (`--clean`) builds

`sti` accepts the following inputs to do a build:

1. Application source: this can be source code, zipped source, a binary, etc
1. Build image: the basis for the new image to build
1. Application image tag: the tag to use for the newly created application image

The basic build process is as follows:

1. `sti` pulls the build image if it is not already present on the system
1. `sti` builds the new image from the supplied build image and source, tagging the output image
   with the supplied tag

### Incremental builds

`sti` automatically detects:

1. Whether a source image is compatible with incremental building
1. Whether an incremental build can be formed when an image is compatible

If the source image is compatible, a prior build already exists, and the `--clean` option is not used,
the workflow is as follows:

1. `sti` creates a new docker container from the prior build image, with a volume in `/tmp/artifacts`
1. `sti` runs `/usr/bin/save-artifacts` in this container - this script is responsible for copying
   the build artifacts into `/tmp/artifacts`.
1. `sti` builds the new output image using the selected build methodology:
    1. The artifacts from the previous build will be in `/tmp/artifacts` during the build
    1. The build image's `/usr/bin/prepare` script is responsible for detecting and using the build
       artifacts

Note the invocation of the `save-artifacts` script; this script is responsible for moving build
dependencies to `/tmp/artifacts`

### Extended builds

Extended builds allow you to execute your build on a build image, then deploy it on a different
runtime image. The workflow for extended builds is as follows:

1. `sti` looks for the previous build image for the tag, `<tag>-build`.
1. If that image exists:
    1. `sti` creates a container from this image and runs `/usr/bin/save-artifacts` in it
1. `sti` creates a build container from the build image with a volume at `/tmp/build`
   and bind-mounts in the artifacts from the prior build, if applicable
1. `sti` runs `/usr/bin/prepare` in the build container - this script is responsible for
   populating `/tmp/build` with the result of the build
1. `sti` builds the output image with the selected build methodology:
    1. The base image will be the runtime image
    1. The output of the source build step will be in `/tmp/src` during the build
    1. The runtime image's `/usr/bin/prepare` script is responsible for being able to deploy the
       artifact in `/tmp/src`
1. If the docker build succeeds, the build container is tagged as `<tag>-build`

You might have noticed that the above workflow describes something like an incremental build.
This behavior can be disabled with the `--clean` option.

### Getting started

#### Dependencies

1. [Docker](http://www.docker.io)
1. [Go](http://golang.org/)

#### Installation

	go get github.com/pmorie/go-sti/sti

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

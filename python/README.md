docker-source-to-images (sti)
=======

This is the python implementation of the source-to-images tool.  

### Getting started

#### Installation

	pip install --user git+https://github.com/openshift/docker-source-to-images

#### Example

You can start using sti right away by using the sample image and application sources in the
`test_sources` directory.  Here's an example that builds a simple HTML app:

	docker pull pmorie/fedora-mock
	sti build git://github.com/pmorie/simple-html fedora-mock sti_app
	docker run -rm -i -p PORT:8080 -t sti_app

### Validating a source image

    sti validate BUILD_IMAGE_TAG [--runtime-image=RUNTIME_IMAGE_TAG] [--incremental] [--url=URL]
        [--timeout=TIMEOUT] [-l LOG_LEVEL]

    Arguments:
        BUILD_IMAGE_TAG        Tag for the Docker image which provides the build and runtime for the application.
        SOURCE_DIR             Directory or GIT repository containing your application sources.
        APP_IMAGE_TAG          Tag for the Docker image which is created by STI. In the case of incremental
                               builds, this tag is also used to identify the previous build of the application.

    Options:
        --runtime-image=RUNTIME_IMAGE_TAG   Tag which identifies an optional Docker image with runtime components but
                                            none of the build dependencies. If provided, the application will be built
                                            with BUILD_IMAGE_TAG and the binaries will be extracted and installed on
                                            the runtime image.
        -l LOG_LEVEL                        Logging level. Default: INFO
        --timeout=TIMEOUT                   Timeout commands if they take too long. Default: 120 seconds.
        --url=URL                           Connect to docker at the specified url [default: unix://var/run/docker.sock]

You can validate that an image is usable as a sti source image as follows:

	sti validate BUILD_IMAGE_TAG

The `--incremental` option to enables validation for incremental builds:

    sti validate BUILD_IMAGE_TAG --incremental

Add the `--runtime-image` option to additionally validate a runtime image for extended builds:

    sti validate BUILD_IMAGE_TAG --runtime-image RUNTIME_IMAGE_TAG

When specifying a runtime image with `sti validate`, the build image is automatically validated for
incremental builds.

### Building a deployable image with sti

    sti build SOURCE_DIR BUILD_IMAGE_TAG APP_IMAGE_TAG [--runtime-image=RUNTIME_IMAGE_TAG] [--clean]
        [--user=USERID] [--url=URL] [--timeout=TIMEOUT] [-e ENV_NAME=VALUE]... [-l LOG_LEVEL]
        [--dir=WORKING_DIR] [--push]

    Arguments:
        BUILD_IMAGE_TAG        Tag for the Docker image which provides the build and runtime for the application.
        SOURCE_DIR             Directory or GIT repository containing your application sources.
        APP_IMAGE_TAG          Tag for the Docker image which is created by STI. In the case of incremental
                               builds, this tag is also used to identify the previous build of the application.

    Options:
        --runtime-image=RUNTIME_IMAGE_TAG   Tag which identifies an optional Docker image with runtime components but
                                            none of the build dependencies. If provided, the application will be built
                                            with BUILD_IMAGE_TAG and the binaries will be extracted and installed on
                                            the runtime image.
        --clean                             Do a clean build, ie. do not perform an incremental build.
        --dir=WORKING_DIR                   Directory where Dockerfiles and other support scripts are created.
                                            (Default: temp dir)
        -l LOG_LEVEL                        Logging level. Default: INFO
        --timeout=TIMEOUT                   Timeout commands if they take too long. Default: 120 seconds.
        --user=USERID                       Perform the build as specified user.
        --url=URL                           Connect to docker at the specified url [default: unix://var/run/docker.sock]

The most basic `sti build` uses a single build image:

    sti build SOURCE_DIR BUILD_IMAGE_TAG APP_IMAGE_TAG

If the build is successful, the built image will be tagged with `APP_IMAGE_TAG`.

If the build image is compatible with incremental builds, `sti build` will look for an image tagged
with `APP_IMAGE_TAG`.  If an image is present with that tag, `sti build` will save the build
artifacts from that image and add them to the build container at `/usr/artifacts` so an image's
`/usr/bin/prepare` script can restore them before building the source.

When using an image that supports incremental builds, you can do a clean build with `--clean`:

    sti build SOURCE_DIR BUILD_IMAGE_TAG APP_IMAGE_TAG --clean

Extended builds allow you to use distinct images for building your sources and deploying them. Use
the `--runtime-image` option perform an extended build targeting a runtime image:

    sti build SOURCE_DIR BUILD_IMAGE_TAG APP_IMAGE_TAG --runtime-image RUNTIME_IMAGE_TAG

When specifying a runtime image, the build image must be compatible with incremental builds.  
`sti build` will look for an image tagged with `<APP_IMAGE_TAG>-build`.  If an image is present with
that tag, `sti build` will save the build artifacts from that image and add them to the build
container at `/usr/artifacts` so the build image's `/usr/bin/prepare` script can restore them before
building the source.  The build image's `/usr/bin/prepare` script is responsible for populating
`/usr/build` with an artifact to be deployed into the runtime container.

After performing the build, a new runtime image is created based on the image tagged with 
`RUNTIME_IMAGE_TAG` with the output of the build in `/usr/src`.  The runtime image's 
`/usr/bin/prepare` script is responsible for detecting and deploying the artifact.  If the build is
successful, two images are tagged:

1. The build image is tagged with `<APP_IMAGE_TAG>-build`
1. The prepared image incorporating the deployed build is tagged with `APP_IMAGE_TAG`

You can do a clean extended build with `--clean`:

    sti build SOURCE_DIR BUILD_IMAGE_TAG APP_IMAGE_TAG --runtime-image RUNTIME_IMAGE_TAG --clean

wharfie
=======

Wharfie is a tool for building reproducable Docker images.  Wharfie produces ready-to-run images by
injecting a user source into a docker image and /preparing/ a new Docker image which incorporates 
the base image and built source, and is ready to use with `docker run`.  Wharfie supports 
incremental builds which re-use previously downloaded dependencies, previously built artifacts, etc.
Interested in learning more?  Read on!

### Basic wharfie builds

Wharfie accepts the following inputs to do a build:

1. Application source: this can be source code, zipped source, a binary, etc.
1. Wharfie source image: the basis for the new image to build
1. Prior build image: optional, a previously built wharfie output image to pull build artifacts from

The build process is as follows:

1. Wharfie pulls the source image if it is not already present on the system
1. Wharfie generates a `Dockerfile` to describe the output image:
    1. Based on the wharfie source image
    1. Adds the application source at `/usr/source` in the container
    1. Calls `/usr/bin/prepare` in the container
    1. Sets the image's default command to `/usr/bin/run`
1. Wharfie builds the new image from the `Dockerfile` using `docker build`

### Anatomy of a wharfie source image

Building wharfie source images is as easy as implementing two scripts.  Wharfie expects the
following scripts in `/usr/bin`:

1. `prepare` : This script is responsible for building and/or deploying the source
1. `run`: This script is responsible for running the deployed source

### Incremental wharfie builds

When you call `wharfie build` with the `--incremental` flag, the build process is as follows:

1. Wharfie pulls the source image if it is not already present on the system
1. Wharfie pulls the incremental build image if it is not already present on the system
1. Wharfie creates a new docker container from the prior image, with a volume in `/usr/artifacts`
1. Wharfie runs `/usr/bin/save-artifact` in this container
1. Wharfie creates a new docker container from the source image, mounting the volumes from the
   incremental build container
1. Wharfie bind-mounts the application source into `/usr/source` in the container
1. Wharfie calls `/usr/bin/prepare` in the container - `prepare` detects previous artifacts and 
   restores them
1. Wharfie commits the container as a new image, setting the new image's command to `/usr/bin/run`

Note the invocation of the `save-artifacts` script; this script is responsible for moving build
dependencies to `/usr/artifacts`

### Getting started

You can start using wharfie right away by using the sample image and application sources in the
`test_sources` directory.  Here's an example that builds a simple HTML app:

	docker build -rm -t fedora-mock test_sources/images/fedora-mock
	wharfie build fedora-mock test_sources/applications/html --tag wharfie_app
	docker run -rm -i -p -t wharfie_app

### Validating a source image

    wharfie validate IMAGE_NAME [--supports-incremental] [--url=URL] [--timeout=TIMEOUT] [-l LOG_LEVEL]

    Arguments:
        IMAGE_NAME      Source image name. Wharfie will pull this image if not available locally.

    Options:
        --supports-incremental          Check for compatibility with incremental builds
        -l LOG_LEVEL                    Logging level. Default: INFO
        --timeout=TIMEOUT               Timeout commands if they take too long. Default: 120 seconds.
        --user=USERID                   Perform the build as specified user.
        --url=URL                       Connect to docker at the specified url [default: unix://var/run/docker.sock]

You can validate that an image is usable as a wharfie source image as follows:

	wharfie validate IMAGE_NAME

Add the `--supports-incremental` option to validate the a source image supports incremental builds:

	wharfie validate IMAGE_NAME --supports-incremental

### Building a deployable image with wharfie

    wharfie build IMAGE_NAME SOURCE_DIR [--tag=BUILD_TAG] [--incremental=PREV_BUILD]
    	[--user=USERID] [--url=URL] [--timeout=TIMEOUT] [-e ENV_NAME=VALUE]... [-l LOG_LEVEL]

    Arguments:
        IMAGE_NAME      Source image name. Wharfie will pull this image if not available locally.
        SOURCE_DIR      Directory containing your application sources.

    Options:
        --incremental=PREV_BUILD        Perform an incremental build. PREV_BUILD specified the previous built image.
        -l LOG_LEVEL                    Logging level. Default: INFO
        --tag=BUILD_TAG                 Tag a successful build with the provided name.
        --timeout=TIMEOUT               Timeout commands if they take too long. Default: 120 seconds.
        --user=USERID                   Perform the build as specified user.
        --url=URL                       Connect to docker at the specified url [default: unix://var/run/docker.sock]

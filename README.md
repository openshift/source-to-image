docker-source-to-images (sti)
=======

source-to-images (`sti`) is a tool for building reproducable Docker images.  source-to-images 
produces ready-to-run images by injecting a user source into a docker image and /preparing/
a new Docker image which incorporates the base image and built source, and is ready to use 
with `docker run`.  source-to-images supports incremental builds which re-use previously 
downloaded dependencies, previously built artifacts, etc. Interested in learning more?  Read on!

### Basic builds

source-to-images accepts the following inputs to do a build:

1. Application source: this can be source code, zipped source, a binary, etc.
1. source-to-images source image: the basis for the new image to build
1. Prior build image: optional, a previously built sti output image to pull build artifacts from

The build process is as follows:

1. source-to-images pulls the source image if it is not already present on the system
1. source-to-images generates a `Dockerfile` to describe the output image:
    1. Based on the sti source image
    1. Adds the application source at `/usr/source` in the container
    1. Calls `/usr/bin/prepare` in the container
    1. Sets the image's default command to `/usr/bin/run`
1. source-to-images builds the new image from the `Dockerfile` using `docker build`

### Philosophy

1. Simplify the process of source + base image -> usable image for most use cases (the 80%)
2. Define and implement a workflow for incremental build that eventually uses only docker primitives
3. Develop tooling that can assist in verifying that two different base images result in the same "docker run" outcome for the same input
4. Use native docker primitives to accomplish this - map out useful improvements to docker build that benefit all image builders

### Anatomy of a source image

Building source images is as easy as implementing two scripts.  source-to-images expects the
following scripts in `/usr/bin`:

1. `prepare` : This script is responsible for building and/or deploying the source
1. `run`: This script is responsible for running the deployed source

### Incremental sti builds

When you call `sti build` with the `--incremental` flag, the build process is as follows:

1. source-to-images pulls the source image if it is not already present on the system
1. source-to-images pulls the incremental build image if it is not already present on the system
1. source-to-images creates a new docker container from the prior image, with a volume in `/usr/artifacts`
1. source-to-images runs `/usr/bin/save-artifact` in this container
1. source-to-images creates a new docker container from the source image, mounting the volumes from the
   incremental build container
1. source-to-images bind-mounts the application source into `/usr/source` in the container
1. source-to-images calls `/usr/bin/prepare` in the container - `prepare` detects previous artifacts and 
   restores them
1. source-to-images commits the container as a new image, setting the new image's command to `/usr/bin/run`

Note the invocation of the `save-artifacts` script; this script is responsible for moving build
dependencies to `/usr/artifacts`

### Getting started

You can start using sti right away by using the sample image and application sources in the
`test_sources` directory.  Here's an example that builds a simple HTML app:

	docker build -rm -t fedora-mock test_sources/images/fedora-mock
	sti build fedora-mock test_sources/applications/html --tag sti_app
	docker run -rm -i -p PORT -t sti_app

### Validating a source image

    sti validate IMAGE_NAME [--supports-incremental] [--url=URL] [--timeout=TIMEOUT] [-l LOG_LEVEL]

    Arguments:
        IMAGE_NAME      Source image name. sti will pull this image if not available locally.

    Options:
        --supports-incremental          Check for compatibility with incremental builds
        -l LOG_LEVEL                    Logging level. Default: INFO
        --timeout=TIMEOUT               Timeout commands if they take too long. Default: 120 seconds.
        --user=USERID                   Perform the build as specified user.
        --url=URL                       Connect to docker at the specified url [default: unix://var/run/docker.sock]

You can validate that an image is usable as a sti source image as follows:

	sti validate IMAGE_NAME

Add the `--supports-incremental` option to validate the a source image supports incremental builds:

	sti validate IMAGE_NAME --supports-incremental

### Building a deployable image with sti

    sti build IMAGE_NAME SOURCE_DIR [--tag=BUILD_TAG] [--incremental=PREV_BUILD]
    	[--user=USERID] [--url=URL] [--timeout=TIMEOUT] [-e ENV_NAME=VALUE]... [-l LOG_LEVEL]

    Arguments:
        IMAGE_NAME      Source image name. sti will pull this image if not available locally.
        SOURCE_DIR      Directory containing your application sources.

    Options:
        --incremental=PREV_BUILD        Perform an incremental build. PREV_BUILD specified the previous built image.
        -l LOG_LEVEL                    Logging level. Default: INFO
        --tag=BUILD_TAG                 Tag a successful build with the provided name.
        --timeout=TIMEOUT               Timeout commands if they take too long. Default: 120 seconds.
        --user=USERID                   Perform the build as specified user.
        --url=URL                       Connect to docker at the specified url [default: unix://var/run/docker.sock]

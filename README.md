docker-source-to-images (sti)
=======

Source-to-images (`sti`) is a tool for building reproducable Docker images.  `sti`
produces ready-to-run images by injecting a user source into a docker image and <i>assembling</i>
a new Docker image which incorporates the base image and built source, and is ready to use
with `docker run`.  `sti` supports incremental builds which re-use previously downloaded 
dependencies, previously built artifacts, etc

Interested in learning more?  Read on!

Don't want to learn more?  Want to just get started NOW?  Check out the getting started instructions [here](go/).

Philosophy
----------

1. Simplify the process of application source + base image -> usable image for most use cases (the
   80%)
2. Define and implement a workflow for incremental build that eventually uses only docker primitives
3. Develop tooling that can assist in verifying that two different base images result in the same
   "docker run" outcome for the same input
4. Use native docker primitives to accomplish this - map out useful improvements to docker that
   benefit all image builders

Anatomy of a source image
-------------------------

Building source images is easy.  `sti` expects you to supply the following scripts to use with an
image:

1. `assemble` : This script is builds and/or deploys the source
2. `run`: This script runs the deployed source
3. `save-artifacts` (optional): This script saves the build context for an incremental build

Build methodologies
-------------------

`sti` builds images with `docker run`.  The workflow is:

1. `sti` creates a container based on the build image. with:
    1. The application source bind-mounted to `/tmp/src`
    1. The build artifacts bind-mounted to `/tmp/artifacts` (if applicable - see incremental builds)
    1. Runs the build image's `assemble` script
1. `sti` starts the container and waits for it to finish running
1. `sti` commits the container, setting the CMD for the output image to be the `run` script and tagging the image with the name provided.

Basic (`--clean`) builds
------------------------

`sti` accepts the following inputs to do a build:

1. Application source: this can be source code, zipped source, a binary, etc
1. Build image: the basis for the new image to build
1. Application image tag: the tag to use for the newly created application image

The basic build process is as follows:

1. `sti` pulls the build image if it is not already present on the system
1. `sti` builds the new image from the supplied build image and source, tagging the output image
   with the supplied tag

Incremental builds
------------------

`sti` automatically detects:

1. Whether a source image is compatible with incremental building
1. Whether an incremental build can be formed when an image is compatible

If a save-artifacts script exists, a prior build already exists, and the `--clean` option is not used,
the workflow is as follows:

1. `sti` creates a new docker container from the prior build image, with a volume in `/tmp/artifacts`
1. `sti` runs `save-artifacts` in this container - this script is responsible for copying
   the build artifacts into `/tmp/artifacts`.
1. `sti` builds the new output image using the selected build methodology:
    1. The artifacts from the previous build will be in `/tmp/artifacts` during the build
    1. The build image's `assemble` script is responsible for detecting and using the build
       artifacts

Note the invocation of the `save-artifacts` script; this script is responsible for moving build
dependencies to `/tmp/artifacts`

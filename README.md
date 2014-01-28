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
1. Wharfie creates a new docker container from the source image
1. Wharfie bind-mounts the application source into `/usr/source` in the container
1. Wharfie calls `/usr/bin/prepare` in the container
1. Wharfie commits the container as a new image, setting the new image's command to `/usr/bin/run`

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
1. Wharfie runs `/usr/bin/restore-artifact` to restore the build context from the prior image
1. Wharfie calls `/usr/bin/prepare` in the container
1. Wharfie commits the container as a new image, setting the new image's command to `/usr/bin/run`

There are two more scripts to implement to support incremental builds:

1. `save-artifact`: This script is responsible for moving build dependencies to `/usr/artifacts`
1. `restore-artifact`: This script is responsible for restoring a build environment from 
`/usr/artifacts`

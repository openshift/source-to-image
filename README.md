source-to-image (sti)
=======

Source-to-image (`sti`) is a tool for building reproducible Docker images.  `sti` produces
ready-to-run images by injecting source code into a docker image and <i>assembling</i>
a new Docker image which incorporates the base image and built source, and is ready to use
with `docker run`.  `sti` supports incremental builds which re-use previously downloaded
dependencies, previously built artifacts, etc.

Interested in learning more?  Read on!

Want to just get started now?  Check out the [instructions](#getting-started).

Philosophy
----------

1. Simplify the process of application source + base image -> usable image for most use cases (the
   80%)
2. Define and implement a workflow for incremental build that eventually uses only docker
   primitives
3. Develop tooling that can assist in verifying that two different base images result in the same
   "docker run" outcome for the same input
4. Use native docker primitives to accomplish this - map out useful improvements to docker that
   benefit all image builders

Anatomy of a source image
-------------------------

Building source images is easy.  `sti` expects you to supply the following scripts to use with an
image:

1. `assemble` : This script builds and/or deploys the source
2. `run`: This script runs the deployed source
3. `save-artifacts` (optional): This script saves the build context for an incremental build
4. `usage` (optional): This script displays builder image usage information

Build methodologies
-------------------

`sti` builds images with `docker run`.  The workflow is:

1. `sti` creates a container based on the build image and passes it a tar file that contains:
    1. The application source in `src`
    1. The build artifacts in `artifacts` (if applicable - see incremental builds)
1. `sti` Starts the container and runs its `assemble` script
1. `sti` Waits for the container to finish
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

1. `sti` creates a new docker container from the prior build image
1. `sti` runs `save-artifacts` in this container - this script is responsible for streaming out
   a tar of the artifacts to stdout
1. `sti` builds the new output image:
    1. The artifacts from the previous build will be in the `artifacts` directory of the tar
       passed to the build
    1. The build image's `assemble` script is responsible for detecting and using the build
       artifacts

Note the invocation of the `save-artifacts` script; this script is responsible for streaming out
dependencies in a tar file

Dependencies
------------

1. [Docker](http://www.docker.io)
1. [Go](http://golang.org/)

Installation
------------

Assuming docker is installed and configured, execute the following commands:

    go get github.com/openshift/source-to-image
    cd ${GOPATH}/src/github.com/openshift/source-to-image
    hack/build-go.sh

Getting Started
---------------

You can start using sti right away with the following test sources and publicly available images:

    sti build git://github.com/pmorie/simple-ruby openshift/ruby-20-centos test-ruby-app
    docker run -rm -i -p :9292 -t test-ruby-app

    sti build git://github.com/bparees/openshift-jee-sample openshift/wildfly-8-centos test-jee-app
    docker run -rm -i -p :8080 -t test-jee-app

Building a Deployable Image
---------------------------

    sti build SOURCE BUILD_IMAGE APP_IMAGE_TAG [flags]

    Available Flags:
         --callbackUrl="": Specify a URL to invoke via HTTP POST upon build completion
         --clean=false: Perform a clean build
     -e, --env="": Specify an environment var NAME=VALUE,NAME2=VALUE2,...
     -r, --ref="": Specify a ref to check-out
         --rm=false: Remove the previous image during incremental builds
         --savetempdir=false: Save the temporary directory used by STI instead of deleting it
     -s, --scripts="": Specify a URL for the assemble and run scripts
     -U, --url="unix:///var/run/docker.sock": Set the url of the docker socket to use
         --verbose=false: Enable verbose output


The most basic `sti build` uses a single build image:

    sti build SOURCE BUILD_IMAGE_TAG APP_IMAGE_TAG

If the build is successful, the built image will be tagged with `APP_IMAGE_TAG`.

If the build image is compatible with incremental builds, `sti build` will look for an image tagged
with `APP_IMAGE_TAG`.  If an image is present with that tag, and a `save-artifacts` script is present, `sti build` will save the build
artifacts from that image and add them to the tar streamed to the container at `/artifacts`.

When using an image that supports incremental builds, you can do a clean build with `--clean`:

    sti build SOURCE BUILD_IMAGE_TAG APP_IMAGE_TAG --clean

Using scripts from a URL
------------------------

You can use a specific set of `assemble`/`run`/
`save-artifacts` scripts with your build image by specifying a URL:

    sti build SOURCE BUILD_IMAGE_TAG APP_IMAGE_TAG -s <url>

If you're creating an image and you want to supply a default set of scripts to use with sti, you
can specify the `STI_SCRIPTS_URL` environment variable in your `Dockerfile`:

    ENV STI_SCRIPTS_URL <url>

Using scripts from an application
----------------------------------

You can also supply assemble/run/save-artifacts scripts in your application source.  The scripts
must be located under `.sti/bin` within the root of your source directory.

Script precedence
-----------------

STI selects which location to use for a given script (assemble, run, and save-artifacts) based on
the following ordering:

1. A script found at the --scripts URL
1. A script found in the application source `.sti/bin` directory
1. A script found at the default image URL (`STI_SCRIPTS_URL`)

Build from a git ref
--------------------

When the source is a git repo, `sti` can check out a git ref before doing the build:

    sti build SOURCE BUILD_IMAGE_TAG APP_IMAGE_TAG -r <ref>

If the ref is invalid or not present in the source repo, the build will fail.

Build callbacks
---------------

Upon completion (or failure) of a build, `sti` can HTTP POST to a URL with information about the
build:

    sti build SOURCE BUILD_IMAGE_TAG APP_IMAGE_TAG --callbackUrl=<url>

The data posted will be of the form:
```
    {
        "payload": "A string containing all build messages",
        "success": true
    }
```

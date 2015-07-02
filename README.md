# source-to-image (sti)

[![GoDoc](https://godoc.org/github.com/openshift/source-to-image?status.png)](https://godoc.org/github.com/openshift/source-to-image)
[![Travis](https://travis-ci.org/openshift/source-to-image.svg?branch=master)](https://travis-ci.org/openshift/source-to-image)


Source-to-image (`sti`) is a tool for building reproducible Docker images. `sti` produces
ready-to-run images by injecting source code into a Docker image and *assembling*
a new Docker image which incorporates the builder image and built source.  The result is then ready to use
with `docker run`. `sti` supports incremental builds which re-use previously downloaded
dependencies, previously built artifacts, etc.

Interested in learning more? Read on!

Want to just get started now? Check out the [instructions](#getting-started).


# Philosophy

1. Simplify the process of application source + builder image -> usable image for most use cases (the
   80%)
1. Define and implement a workflow for incremental builds that eventually uses only Docker
   primitives
1. Develop tooling that can assist in verifying that two different builder images result in the same
   `docker run` outcome for the same input
1. Use native Docker primitives to accomplish this - map out useful improvements to Docker that
   benefit all image builders


# Anatomy of a builder image

Creating builder images is easy. `sti` looks for you to supply the following scripts to use with an
image:

1. `assemble` - builds and/or deploys the source
1. `run`- runs the assembled artifacts
1. `save-artifacts` (optional) - captures the artifacts from a previous build into the next incremental build
1. `usage` (optional) - displays builder image usage information

Additionally for the best user experience and optimized `sti` operation we suggest images
to have `/bin/sh` and `tar` commands available.

Users can also set extra environment variables in the application source code. 
They are passed to the build, and the `assemble` script consumes them. All
environment variables are also present in the output application image. These
variables are defined in the `.sti/environment` file inside the application sources.
The format of this file is a simple key-value, for example:

```
FOO=bar
```

In this case, the value of `FOO` environment variable will be set to `bar`.

See [here](https://github.com/openshift/source-to-image/blob/master/docs/builder_image.md) for a detailed description of the requirements and scripts along with examples of builder images.



# Build workflow

The `sti build` workflow is:

1. `sti` creates a container based on the build image and passes it a tar file that contains:
    1. The application source in `src`
    1. The build artifacts in `artifacts` (if applicable - see [incremental builds](#incremental-builds))
1. `sti` sets the environment variables from `.sti/environment` (optional)
1. `sti` starts the container and runs its `assemble` script
1. `sti` waits for the container to finish
1. `sti` commits the container, setting the CMD for the output image to be the `run` script and tagging the image with the name provided.

# Using ONBUILD images

In case you want to use one of the official Docker language stack images for
your build you don't have do anything extra. STI is capable of recognizing the
Docker image with [ONBUILD](https://docs.docker.com/reference/builder/#onbuild) instructions and choosing the OnBuild strategy. This
strategy will trigger all ONBUILD instructions and execute the assemble script
(if it exists) as the last instruction.

Since the ONBUILD images usually don't provide any entrypoint, in order to use
this build strategy you will have to provide one. You can either include the 'run',
'start' or 'execute' script in your application source root folder or you can
specify a valid STI script URL and the 'run' script will be fetched and set as
an entrypoint in that case.

## Incremental builds

`sti` automatically detects:

* Whether a builder image is compatible with incremental building
* Whether a previous image exists, with the same name as the output name for this build

If a `save-artifacts` script exists, a prior image already exists, and the `--incremental=true` option is used, the workflow is as follows:

1. `sti` creates a new Docker container from the prior build image
1. `sti` runs `save-artifacts` in this container - this script is responsible for streaming out
   a tar of the artifacts to stdout
1. `sti` builds the new output image:
    1. The artifacts from the previous build will be in the `artifacts` directory of the tar
       passed to the build
    1. The build image's `assemble` script is responsible for detecting and using the build
       artifacts

**NOTE**: The `save-artifacts` script is responsible for streaming out dependencies in a tar file.


# Dependencies

1. [Docker](http://www.docker.io) >= 1.6
1. [Go](http://golang.org/) >= 1.4


# Installation

Assuming Go and Docker are installed and configured, execute the following commands:

```
$ go get github.com/openshift/source-to-image
$ cd ${GOPATH}/src/github.com/openshift/source-to-image
$ export PATH=$PATH:${GOPATH}/src/github.com/openshift/source-to-image/_output/local/go/bin/
$ hack/build-go.sh
```

# Security

Since the `sti` command uses the Docker client library, it has to run in the same 
security context as the `docker` command. For some systems, it is enough to add 
yourself into the 'docker' group to be able to work with Docker as 'non-root'. 
In the latest versions of Fedora/RHEL, it is recommended to use the `sudo` command 
as this way is more auditable and secure.

If you are using the `sudo docker` command already, then you will have to also use
`sudo sti` to give STI permission to work with Docker directly.

# Getting Started

You can start using `sti` right away (see [releases](https://github.com/openshift/source-to-image/releases))
with the following test sources and publicly available images:

```
$ sti build git://github.com/pmorie/simple-ruby openshift/ruby-20-centos7 test-ruby-app
$ docker run --rm -i -p :9292 -t test-ruby-app
```

```
$ sti build git://github.com/bparees/openshift-jee-sample openshift/wildfly-8-centos test-jee-app
$ docker run --rm -i -p :8080 -t test-jee-app
```

Interested in more advanced `sti` usage? See [here](https://github.com/openshift/source-to-image/blob/master/docs/cli.md)
for detailed descriptions of the different CLI commands with examples.

# source-to-image (sti)

[![GoDoc](https://godoc.org/github.com/openshift/source-to-image?status.png)](https://godoc.org/github.com/openshift/source-to-image)
[![Travis](https://travis-ci.org/openshift/source-to-image.svg?branch=master)](https://travis-ci.org/openshift/source-to-image)


Source-to-image (`sti`) is a tool for building reproducible docker images. `sti` produces
ready-to-run images by injecting source code into a docker image and *assembling*
a new docker image which incorporates the builder image and built source, and is ready to use
with `docker run`. `sti` supports incremental builds which re-use previously downloaded
dependencies, previously built artifacts, etc.

Interested in learning more? Read on!

Want to just get started now? Check out the [instructions](#getting-started).


# Philosophy

1. Simplify the process of application source + builder image -> usable image for most use cases (the
   80%)
1. Define and implement a workflow for incremental build that eventually uses only docker
   primitives
1. Develop tooling that can assist in verifying that two different builder images result in the same
   "docker run" outcome for the same input
1. Use native docker primitives to accomplish this - map out useful improvements to docker that
   benefit all image builders


# Anatomy of a builder image

Creating builder images is easy. `sti` expects you to supply the following scripts to use with an
image:

1. `assemble` - builds and/or deploys the source
1. `run`- runs the assembled artifacts
1. `save-artifacts` (optional) - captures the artifacts from a previous build into the next incremental build
1. `usage` (optional) - displays builder image usage information

Additionally for the best user experience and optimized sti operation we suggest image
to have `/bin/sh` and tar command inside.

For detailed description of the requirements and the scripts along with examples see
[builder_image.md](https://github.com/openshift/source-to-image/blob/master/docs/builder_image.md)


# Build workflow

`sti build` workflow is:

1. `sti` creates a container based on the build image and passes it a tar file that contains:
    1. The application source in `src`
    1. The build artifacts in `artifacts` (if applicable - see [incremental builds](#incremental-builds))
1. `sti` starts the container and runs its `assemble` script
1. `sti` waits for the container to finish
1. `sti` commits the container, setting the CMD for the output image to be the `run` script and tagging the image with the name provided.

# Using ONBUILD images

In case you want to use one of the official Docker language stack images for
your build you don't have do anything extra. The STI is capable to recognize the
Docker image with ONBUILD instructions and choose the OnBuild strategy. This
strategy will trigger all ONBUILD instructions and execute the assemble script
(if it exists) as the last instruction.

Since the ONBUILD images usually don't provide any entrypoint, in order to use
this build strategy you have to provide it. You can either include the 'run',
'start' or 'execute' script in your application source root folder or you can
specify a valid STI scripts URL and the 'run' script will be fetched and set as
an entrypoint in that case.

## Incremental builds

`sti` automatically detects:

* Whether a builder image is compatible with incremental building
* Whether a previous image exists, with the same name as the output name for this build

If a `save-artifacts` script exists, a prior image already exists, and the `--clean` option is not used,
the workflow is as follows:

1. `sti` creates a new docker container from the prior build image
1. `sti` runs `save-artifacts` in this container - this script is responsible for streaming out
   a tar of the artifacts to stdout
1. `sti` builds the new output image:
    1. The artifacts from the previous build will be in the `artifacts` directory of the tar
       passed to the build
    1. The build image's `assemble` script is responsible for detecting and using the build
       artifacts

**NOTE**: The `save-artifacts` script is responsible for streaming out dependencies in a tar file.


# Dependencies

1. [Docker](http://www.docker.io)
1. [Go](http://golang.org/)


# Installation

Assuming go and docker are installed and configured, execute the following commands:

```
$ go get github.com/openshift/source-to-image
$ cd ${GOPATH}/src/github.com/openshift/source-to-image
$ export PATH=$PATH:${GOPATH}/src/github.com/openshift/source-to-image/_output/local/go/bin/
$ hack/build-go.sh
```


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

Interested in more advanced `sti` usage? See [cli.md](https://github.com/openshift/source-to-image/blob/master/docs/cli.md)
for detailed CLI description with examples.

# source-to-image (s2i)

[![GoDoc](https://godoc.org/github.com/openshift/source-to-image?status.png)](https://godoc.org/github.com/openshift/source-to-image)
[![Travis](https://travis-ci.org/openshift/source-to-image.svg?branch=master)](https://travis-ci.org/openshift/source-to-image)


Source-to-image (`s2i`) is a tool for building reproducible Docker images. `s2i` produces
ready-to-run images by injecting source code into a Docker image and *assembling*
a new Docker image which incorporates the builder image and built source.  The result is then ready to use
with `docker run`. `s2i` supports incremental builds which re-use previously downloaded
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

See a practical tutorial on how to create a builder image [here](examples/README.md)

Creating builder images is easy. `s2i` looks for you to supply the following scripts to use with an
image:

1. `assemble` - builds and/or deploys the source
1. `run`- runs the assembled artifacts
1. `save-artifacts` (optional) - captures the artifacts from a previous build into the next incremental build
1. `usage` (optional) - displays builder image usage information

Additionally for the best user experience and optimized `s2i` operation we suggest images
to have `/bin/sh` and `tar` commands available.

Filtering the contents of the source tree is possible if the user supplies a
`.s2iignore` file in the root directory of the source repository, where `.s2iignore` contains regular
expressions that capture the set of files and directories you want filtered from the image s2i produces.

Specifically:

1. Specify one rule per line, with each line terminating in `\n`.
1. Filepaths are appended to the absolute path of the  root of the source tree (either the local directory supplied, or the target destination of the clone of the remote source repository s2i creates).
1. Wildcards and globbing (file name expansion) leverage Go's `filepath.Match` and `filepath.Glob` functions.
1. Search is not recursive.  Subdirectory paths must be specified (though wildcards and regular expressions can be used in the subdirectory specifications).
1. If the first character is the `#` character, the line is treated as a comment.
1. If the first character is the `!`, the rule is an exception rule, and can undo candidates selected for filtering by prior rules (but only prior rules).

Here are some examples to help illustrate:

With specifying subdirectories, the `*/temp*` rule prevents the filtering of any files starting with `temp` that are in any subdirectory that is immediately (or one level) below the root directory.
And the `*/*/temp*` rule prevents the filtering of any files starting with `temp` that are in any subdirectory that is two levels below the root directory.

Next, to illustrate exception rules, first consider the following example snippet of a `.s2iignore` file:


	*.md
	!README.md


With this exception rule example, README.md will not be filtered, and remain in the image s2i produces.  However, with this snippet:


	!README.md
	*.md


README.md, if filtered by any prior rules, but then put back in by `!README.md`, would be filtered, and not part of the resulting image s2i produces.  Since `*.md` follows `!README.md`, `*.md` takes precedence.

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

The `s2i build` workflow is:

1. `s2i` creates a container based on the build image and passes it a tar file that contains:
    1. The application source in `src`, excluding any files selected by `.s2iignore`
    1. The build artifacts in `artifacts` (if applicable - see [incremental builds](#incremental-builds))
1. `s2i` sets the environment variables from `.sti/environment` (optional)
1. `s2i` starts the container and runs its `assemble` script
1. `s2i` waits for the container to finish
1. `s2i` commits the container, setting the CMD for the output image to be the `run` script and tagging the image with the name provided.

# Using ONBUILD images

In case you want to use one of the official Docker language stack images for
your build you don't have do anything extra. S2I is capable of recognizing the
Docker image with [ONBUILD](https://docs.docker.com/reference/builder/#onbuild) instructions and choosing the OnBuild strategy. This
strategy will trigger all ONBUILD instructions and execute the assemble script
(if it exists) as the last instruction.

Since the ONBUILD images usually don't provide any entrypoint, in order to use
this build strategy you will have to provide one. You can either include the 'run',
'start' or 'execute' script in your application source root folder or you can
specify a valid S2I script URL and the 'run' script will be fetched and set as
an entrypoint in that case.

## Incremental builds

`s2i` automatically detects:

* Whether a builder image is compatible with incremental building
* Whether a previous image exists, with the same name as the output name for this build

If a `save-artifacts` script exists, a prior image already exists, and the `--incremental=true` option is used, the workflow is as follows:

1. `s2i` creates a new Docker container from the prior build image
1. `s2i` runs `save-artifacts` in this container - this script is responsible for streaming out
   a tar of the artifacts to stdout
1. `s2i` builds the new output image:
    1. The artifacts from the previous build will be in the `artifacts` directory of the tar
       passed to the build
    1. The build image's `assemble` script is responsible for detecting and using the build
       artifacts

**NOTE**: The `save-artifacts` script is responsible for streaming out dependencies in a tar file.


# Dependencies

1. [Docker](http://www.docker.io) >= 1.6
1. [Go](http://golang.org/) >= 1.4
1. (optional) [Git](https://git-scm.com/)


# Installation

Assuming Go and Docker are installed and configured, execute the following commands:

```
$ go get github.com/openshift/source-to-image
$ cd ${GOPATH}/src/github.com/openshift/source-to-image
$ export PATH=$PATH:${GOPATH}/src/github.com/openshift/source-to-image/_output/local/bin/linux/amd64/
$ hack/build-go.sh
```

# Security

Since the `s2i` command uses the Docker client library, it has to run in the same
security context as the `docker` command. For some systems, it is enough to add
yourself into the 'docker' group to be able to work with Docker as 'non-root'.
In the latest versions of Fedora/RHEL, it is recommended to use the `sudo` command
as this way is more auditable and secure.

If you are using the `sudo docker` command already, then you will have to also use
`sudo s2i` to give S2I permission to work with Docker directly.

Be aware that being a member of the 'docker' group effectively grants root access,
as described [here](https://github.com/docker/docker/issues/9976).

# Getting Started

You can start using `s2i` right away (see [releases](https://github.com/openshift/source-to-image/releases))
with the following test sources and publicly available images:

```
$ s2i build git://github.com/pmorie/simple-ruby openshift/ruby-20-centos7 test-ruby-app
$ docker run --rm -i -p :8080 -t test-ruby-app
```

```
$ s2i build git://github.com/bparees/openshift-jee-sample openshift/wildfly-8-centos test-jee-app
$ docker run --rm -i -p :8080 -t test-jee-app
```

Interested in more advanced `s2i` usage? See [here](https://github.com/openshift/source-to-image/blob/master/docs/cli.md)
for detailed descriptions of the different CLI commands with examples.

Running into some issues and need some advice debugging?  Peruse [here](https://github.com/openshift/source-to-image/blob/master/docs/debugging-s2i.md) for some tips.

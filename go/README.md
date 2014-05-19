go-sti
======

This is the golang implementation of the source-to-images tool.

Dependencies
------------

1. [Docker](http://www.docker.io)
1. [Go](http://golang.org/)

Installation
------------

	go get github.com/openshift/docker-source-to-images/go

Getting Started
---------------

You can start using sti right away with the following test sources and publicly available images:

    sti build git://github.com/pmorie/simple-ruby openshift/ruby-19-centos test-ruby-app
    docker run -rm -i -p :9292 -t test-ruby-app

    sti build git://github.com/pmorie/simple-ruby openshift/buildpack-ubuntu test-foreman-app \
    -e 'BUILDPACK_URL=https://github.com/heroku/heroku-buildpack-ruby.git'
    docker run -rm -i -p :5000 -t test-foreman-app

    sti build git://github.com/pmorie/simple-html pmorie/fedora-mock test-html-app
    docker run -rm -i -p :8080 -t sti_app

Building a Deployable Image
---------------------------

    sti build SOURCE BUILD_IMAGE APP_IMAGE_TAG [flags]

    Available Flags:
         --callbackUrl="": Specify a URL to invoke via HTTP POST upon build completion
         --clean=false: Perform a clean build
         --dir="tempdir": Directory where generated Dockerfiles and other support scripts are created
     -e, --env="": Specify an environment var NAME=VALUE,NAME2=VALUE2,...
     -r, --ref="": Specify a ref to check-out
     -s, --scripts="": Specify a URL for the assemble, run, and save-artifacts scripts
     -R, --runtime="": Set the runtime image to use
     -U, --url="unix:///var/run/docker.sock": Set the url of the docker socket to use
         --verbose=false: Enable verbose output


The most basic `sti build` uses a single build image:

    sti build SOURCE BUILD_IMAGE_TAG APP_IMAGE_TAG

If the build is successful, the built image will be tagged with `APP_IMAGE_TAG`.

If the build image is compatible with incremental builds, `sti build` will look for an image tagged
with `APP_IMAGE_TAG`.  If an image is present with that tag, and a `save-artifacts` script is present, `sti build` will save the build
artifacts from that image and add them to the build container at `/tmp/artifacts` so the `assemble` script can restore them before building the source.

When using an image that supports incremental builds, you can do a clean build with `--clean`:

    sti build SOURCE BUILD_IMAGE_TAG APP_IMAGE_TAG --clean

Using scripts from a URL
------------------------

You can use any set of `assemble`/`run`/`save-artifacts` scripts you want with an image by specifying a url:

    sti build SOURCE BUILD_IMAGE_TAG APP_IMAGE_TAG -s <url>

To provide a default set of images to use with your image, you can specify the `STI_SCRIPTS_URL` environment var in
your `Dockerfile`:

    ENV STI_SCRIPTS_URL <url>

Using scripts from an application
----------------------------------

You can also supply assemble/run/save-artifacts scripts in your application source.  The scripts must be located
under `.sti/bin` within the root of your source directory.

Script precedence
-----------------

STI selects which location to use for a given script (assemble, run, and save-artifacts) based on the following ordering:

1. A script found at the --scripts URL
1. A script found in the application source `.sti/bin` directory
1. A script found at the default image URL (STI_SCRIPTS_URL)


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

    {
        "payload": "A string containing all build messages",
        "success": true
    }

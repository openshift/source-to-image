# sti builder image requirements

The main advantage of using sti for building reproducible docker images is ease of use
for developers. To meet that criteria you, as a builder image author, should be aware
of the two basic requirements for the best possible sti performance, these are:

* [required image contents](#required-image-contents)
* [sti scripts](#sti-scripts)


# Required image contents

The build process consists of three fundamental elements which are combined into
final docker image.  These three are: sources, scripts and builder image. During the
build process sti must place sources and scripts inside that builder image. To do
so sti creates a tar file containing the two and then streams that file into the
builder image. Before executing the `assemble` script, sti untars that file and places
its contents into the destination specified with either the `--destination` flag or `io.s2i.destination`
label from the builder image (default destination is `/tmp`). For this
to happen your image must supply the tar archiving utility (command `tar` available in `$PATH`)
and a command line interpreter (command `/bin/sh`). Doing so will allow your image to
use the fastest possible build path, because in all other cases when either
`tar` or `/bin/sh` are not available, sti build will be forced to perform an additional
docker build to put both sources and scripts inside the image and only then run the
usual sti build procedure (sti will do this automatically if tar or /bin/sh are not found).
See the following diagram for how build workflow looks like:

![sti workflow](./sti-flow.png "sti workflow")

\* Run build's responsibility is to untar the sources, scripts and artifacts (if such
exist) and invoke the `assemble` script. If this is the second run (due to catching any `tar`/`/bin/sh` related
errors) it's responsibility is only to invoke the `assemble` script, since both scripts and sources are already there.


# sti scripts

`sti` expects you, as the builder image author, to supply the following scripts:

* required:
    * [assemble](#assemble)
    * [run](#run)
* optional:
    * [save-artifacts](#save-artifacts)
    * [usage](#usage)
    * [test/run](#test/run)

All of the scripts can be written in any programming language, as long as the scripts
are executable inside the builder image. STI supports multiple options providing
`assemble`/`run`/`save-artifacts` scripts. All of these locations are checked on
each build in the following order:

1. A script found at the `--scripts-url` URL
1. A script found in the application source `.sti/bin` directory
1. A script found at the default image URL (`io.openshift.s2i.scripts-url` label)

Both the `io.openshift.s2i.scripts-url` label specified in the image and `--scripts-url` flag
can take one of the following forms:

* `image://path_to_scripts_dir` - absolute path inside the image to a directory where the STI scripts are located
* `file://path_to_scripts_dir` - relative or absolute path to a directory on the host where the STI scripts are located
* `http(s)://path_to_scripts_dir` - URL to a directory where the STI scripts are located

**NOTE**: In the case where the scripts are already placed inside the image (using `--scripts-url`
or `io.openshift.s2i.scripts-url` with value `image:///path/in/image`) then setting `--destination`
or `io.openshift.s2i.destination` label applies only to sources and artifacts.

## assemble

The `assemble` script is responsible for building the application artifacts from source
and placing them into appropriate directories inside the image. The workflow for `assemble` is:

1. Restore build artifacts (in case you want to support incremental builds, make sure
   to define [save-artifacts](#save-artifacts)) as well.
1. Place the application source in the desired destination.
1. Build application artifacts.
1. Install the artifacts into locations appropriate for running.

#### Example `assemble` script:

**NOTE**: All the examples are written in [Bash](http://www.gnu.org/software/bash/)
and it is assumed all the tar contents unpack into the `/tmp/sti` directory.

```
#!/bin/bash

# restore build artifacts
if [ "$(ls /tmp/sti/artifacts/ 2>/dev/null)" ]; then
    mv /tmp/sti/artifacts/* $HOME/.
fi

# move the application source
mv /tmp/sti/src $HOME/src

# build application artifacts
pushd ${HOME}
make all

# install the artifacts
make install
popd
```

## run

The `run` script is responsible for executing your application.

#### Example `run` script:

```
#!/bin/bash

# run the application
/opt/application/run.sh
```

## save-artifacts

The `save-artifacts` script is responsible for gathering all the dependencies into a tar file and streaming it to the standard output.  The existance of this can speed up the following build processes (eg. for Ruby - gems installed by Bundler, for Java - `.m2` contents, etc.).

#### Example `save-artifacts` script:

```
#!/bin/bash

pushd ${HOME}
if [ -d deps ]; then
    # all deps contents to tar stream
    tar cf - deps
fi
popd

```

## usage

The `usage` script is for you, as the builder image author, to inform the user
how to properly use your image.

#### Example `usage` script:

```
#!/bin/bash

# inform the user how to use the image
cat <<EOF
This is a STI sample builder image, to use it, install
https://github.com/openshift/source-to-image
EOF
```

## test/run

The `test/run` script is for you, as the builder image author, to create a simple
process to checks if the image is working correctly. The proposed flow of that process
should be following:

1. Build the image.
1. Run the image to verify `usage` script.
1. Run `sti build` to verify `assemble` script.
1. (optional) Run `sti build` once more to verify `save-artifacts` script and
   `assemble`'s restore artifacts functionality.
1. Run the image to verify the test application is working.

**NOTE** The suggested place to put your test application which should be built by your
`test/run` script is `test/test-app` in your image repository, see
[sti create](https://github.com/openshift/source-to-image/blob/master/docs/cli.md#sti-create).

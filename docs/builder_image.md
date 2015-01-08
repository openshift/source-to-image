# sti builder image requirements

The main advantage of using sti for building reproducible docker image ease of use
for developer. To meet that criteria you, as a builder image author, should be aware
of the two basic requirements for the best possible sti performance, these are:

* [required image contents](#required-image-contents)
* [sti scripts](#sti-scripts)


# Required image contents

The build process consists of three fundamental elements which are combined into
final docker image, the three are: sources, scripts and builder image. During the
build process sti must place sources and scripts inside that builder image. To do
so sti creates a tar file containing the two and then streams that file into the
builder image. Before executing `assemble` script, sti untars that file and places
its contents into the location specified with `--location` flag or `STI_LOCATION`
environment variable from the builder image (default location is `/tmp`). For this
to happen your image must supply tar archiving utility (command `tar` available in `$PATH`)
and command line interpreter (command `/bin/sh`). Doing so will allow your image to
use the fastest possible build path, because in all other cases when either
`tar` or `/bin/sh` is not available, sti build will be forced to perform an additional
docker build to put both sources and scripts inside the image and only then run the
usual sti build procedure (sti will do this automatically if tar and /bin/sh are not found).
See the following diagram for how build workflow looks like:

![sti workflow](./sti-flow.png "sti workflow")

\* Run build's responsibility is to untar the sources, scripts and artifacts (if such
exist) and invoke `assemble` script. If this is second run (after catching `tar`/`/bin/sh`
error) it's responsible only for invoking `assemble` script, since both scripts and
sources are already there.


# sti scripts

`sti` expects you, as the builder image author, to supply the following scripts:

* required:
    * [assemble](#assemble)
    * [run](#run)
* optional:
    * [save-artifacts](#save-artifacts)
    * [usage](#usage)

All of the scripts can be written in any programming language, as long as it is
executable inside the builder image.

## assemble

The `assemble` script is responsible for building the application artifacts from source,
and place them into appropriate directories inside the image. The workflow for `assemble` is:

1. Restore build artifacts (in case you want to support incremental builds, make sure
   to define [save-artifacts](#save-artifacts)) as well.
1. Place the application source in desired location.
1. Build application artifacts.
1. Install the artifacts into locations appropriate for running.

#### Example `assemble` script:

**NOTE**: All the examples are written in [Bash](http://www.gnu.org/software/bash/)
and it is assumed all the tar contents is unpacked into `/tmp/sti` directory.

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

The `save-artifacts` script is responsible for gathering all the dependencies which
existence can speed up the following build processes (eg. for Ruby - Gemfiles,
for Java - `.m2` contents, etc.) into a tar file and stream it to the standard output.

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

# sti command line interface

This document describes thoroughly all `sti` subcommands and flags with explanation
of their purpose as well as an example usage.

Currently `sti` has five subcommands, each of which will be described in the
following sections of this document:

* [create](#sti-create)
* [build](#sti-build)
* [usage](#sti-usage)
* [version](#sti-version)
* [help](#sti-help)

Before diving into each of the aforementioned commands, let's have a closer look
at common flags that can be used with all of the subcommands.

| Name                       | Description                                             |
|:-------------------------- |:--------------------------------------------------------|
| `-h (--help)`              | Display help for the specified command |
| `--loglevel`               | Set the level of log output (0-5) (see [Log levels](#log-levels))|
| `-U (--url)`               | URL of the Docker socket to use (default: `unix:///var/run/docker.sock`) |

#### Log levels

There are four log levels:
* Level `0` - produces output from containers running `assemble` script and all encountered errors
* Level `1` - produces basic information about the executed process
* Level `2` - produces very detailed information about the executed process
* Level `3` - produces very detailed information about the executed process, along with listing tar contents
* Level `4` - currently produces same information as level `3` 
* Level `5` - produces very detailed information about the executed process, lists tar contents, Docker Registry credentials, and copied source files

**NOTE**: All of the commands and flags are case sensitive!

# sti create

The `sti create` command is responsible for bootstrapping a new STI enabled
image repository. This command will generate a skeleton `.sti` directory and
populate it with sample STI scripts you can start hacking on.

Usage:

```
$ sti create <image name> <destination directory>
```

# sti build

The `sti build` command is responsible for building the Docker image by combining
the specified builder image and sources. The resulting image will be named according
to the tag parameter. 

Usage:
```
$ sti build <source location> <builder image> [<tag>] [flags]
```
The build command parameters are defined as follows:

1. `source location` - the URL of a GIT repository or a local path to the source code
1. `builder image` - the Docker image to be used in building the final image
1. `tag` - the name of the final Docker image (if provided)

If the build image is compatible with incremental builds, `sti build` will look for
an image tagged with the same name. If an image is present with that tag and a
`save-artifacts` script is present in the scripts directory, `sti build` will save the build artifacts from
that image and add them to the tar streamed to the container into `/artifacts`.

#### Build flags

| Name                       | Description                                             |
|:-------------------------- |:--------------------------------------------------------|
| `--callback-url`           | URL to be invoked after a successful build (see [Callback URL](#callback-url)) |
| `-d (--destination)`       | Location where the scripts and sources will be placed prior doing build (see [STI Scripts](https://github.com/openshift/source-to-image/blob/master/docs/builder_image.md#sti-scripts)) |
| `--dockercfg-path`         | The path to the Docker configuration file |
| `--incremental`            | Try to perform an incremental build |
| `-e (--env)`               | Environment variables to be passed to the builder eg. `NAME=VALUE,NAME2=VALUE2,...` |
| `--force-pull`             | Always pull the builder image, even if it is present locally (defaults to true) |
| `-r (--ref)`               | A branch/tag that the build should use instead of MASTER (applies only to GIT source) |
| `--rm`                     | Remove the previous image during incremental builds |
| `--save-temp-dir`          | Save the working directory used for fetching scripts and sources |
| `--context-dir`            | Specify the directory containing your application (if not located within the root path) |
| `-s (--scripts-url)`       | URL of STI scripts (see [STI Scripts](https://github.com/openshift/source-to-image/blob/master/docs/builder_image.md#sti-scripts)) |
| `-q (--quiet)`             | Operate quietly, suppressing all non-error output |

#### Context directory

In the case where your application resides in a directory other than your repository root
folder, you can specify that directory using the `--context-dir` parameter. The 
specified directory will be used as your application root folder.

#### Callback URL

Upon completion (or failure) of a build, `sti` can execute a HTTP POST to a URL with information
about the build:

* `success` - flag indicating the result of the build process (`true` or `false`)
* `payload` - list of messages from the build process

Example: data posted will be in the form:
```
{
    "payload": "A string containing all build messages",
    "success": true
}
```

#### Example Usage

Build a Ruby application from a GIT source, using the official `ruby-20-centos7` builder
image, the resulting image will be named `ruby-app`:

```
$ sti build git://github.com/mfojtik/sinatra-app-example openshift/ruby-20-centos7 ruby-app
```

Build a Node.js application from a local directory, using a local image, the resulting
image will be named `nodejs-app`:

```
$ sti build --force-pull=false ~/nodejs-app local-nodejs-builder nodejs-app
```

Build a Java application from a GIT source, using the official `wildfly-8-centos`
builder image but overriding the scripts URL from local directory.  The resulting
image will be named `java-app`:

```
$ sti build --scripts-url=file://stiscripts git://github.com/bparees/openshift-jee-sample openshift/wildfly-8-centos java-app
```

Build a Ruby application from a GIT source, specifying `ref`, and using the official
`ruby-20-centos7` builder image.  The resulting image will be named `ruby-app`:

```
$ sti build --ref=my-branch git://github.com/mfojtik/sinatra-app-example openshift/ruby-20-centos7 ruby-app
```

***NOTE:*** If the ref is invalid or not present in the source repository then the build will fail.

Build a Ruby application from a GIT source, overriding the scripts URL from a local directory,
and specifying the scripts and sources be placed in `/opt` directory:

```
$ sti build --scripts-url=file://stiscripts --destination=/opt git://github.com/mfojtik/sinatra-app-example openshift/ruby-20-centos7 ruby-app
```

# sti rebuild

The `sti rebuild` command is used to rebuild an image already built using S2I,
or the image that contains the required S2I labels.
The rebuild will read the S2I labels and automatically set the builder image,
source repository and other configuration options used to build the previous
image according to the stored labels values.

Optionally, you can set the new image name as a second argument to the rebuild
command.

Usage:

```
$ sti rebuild <image name> [<new-tag-name>]
```


# sti usage

The `sti usage` command starts a container and runs the `usage` script which prints
information about the builder image. This command expects `builder image` name as
the only parameter.

Usage:
```
$ sti usage <builder image> [flags]
```

#### Usage flags

| Name                       | Description                                             |
|:-------------------------- |:--------------------------------------------------------|
| `-d (--destination)`       | Location where the scripts and sources will be placed prior invoking usage (see [STI Scripts](https://github.com/openshift/source-to-image/blob/master/docs/builder_image.md#sti-scripts))|
| `-e (--env)`               | Environment variables passed to the builder eg. `NAME=VALUE,NAME2=VALUE2,...`) |
| `--force-pull`             | Always pull the builder image, even if it is present locally |
| `--save-temp-dir`          | Save the working directory used for fetching scripts and sources |
| `-s (--scripts-url)`       | URL of STI scripts (see [Scripts URL](https://github.com/openshift/source-to-image/blob/master/docs/builder_image.md#sti-scripts))|

#### Example Usage

Print the official `ruby-20-centos7` builder image usage:
```
$ sti usage openshift/ruby-20-centos7
```


# sti version

The `sti version` command prints the version of STI currently installed.


# sti help

The `sti help` command prints help either for the `sti` itself or for the specified
subcommand.

### Example Usage

Print the help page for the build command:
```
$ sti help build
```

***Note:*** You can also accomplish this with:
```
$ sti build --help
```
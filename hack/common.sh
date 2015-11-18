#!/bin/bash

# This script provides common script functions for the hacks
# Requires STI_ROOT to be set

set -o errexit
set -o nounset
set -o pipefail

# The root of the build/dist directory
STI_ROOT=$(
  unset CDPATH
  sti_root=$(dirname "${BASH_SOURCE}")/..
  cd "${sti_root}"
  pwd
)

STI_OUTPUT_SUBPATH="${STI_OUTPUT_SUBPATH:-_output/local}"
STI_OUTPUT="${STI_ROOT}/${STI_OUTPUT_SUBPATH}"
STI_OUTPUT_BINPATH="${STI_OUTPUT}/bin"
STI_LOCAL_BINPATH="${STI_OUTPUT}/go/bin"
STI_LOCAL_RELEASEPATH="${STI_OUTPUT}/releases"

readonly STI_GO_PACKAGE=github.com/openshift/source-to-image
readonly STI_GOPATH="${STI_OUTPUT}/go"

readonly STI_CROSS_COMPILE_PLATFORMS=(
  linux/amd64
  darwin/amd64
  windows/amd64
  linux/386
)
readonly STI_CROSS_COMPILE_TARGETS=(
  cmd/s2i
)
readonly STI_CROSS_COMPILE_BINARIES=("${STI_CROSS_COMPILE_TARGETS[@]##*/}")

readonly STI_ALL_TARGETS=(
  "${STI_CROSS_COMPILE_TARGETS[@]}"
)

readonly STI_BINARY_SYMLINKS=(
  sti
)
readonly STI_BINARY_COPY=(
  sti
)
readonly STI_BINARY_RELEASE_WINDOWS=(
  sti.exe
  s2i.exe
)

# sti::build::binaries_from_targets take a list of build targets and return the
# full go package to be built
sti::build::binaries_from_targets() {
  local target
  for target; do
    echo "${STI_GO_PACKAGE}/${target}"
  done
}

# Asks golang what it thinks the host platform is.  The go tool chain does some
# slightly different things when the target platform matches the host platform.
sti::build::host_platform() {
  echo "$(go env GOHOSTOS)/$(go env GOHOSTARCH)"
}


# Build binaries targets specified
#
# Input:
#   $@ - targets and go flags.  If no targets are set then all binaries targets
#     are built.
#   STI_BUILD_PLATFORMS - Incoming variable of targets to build for.  If unset
#     then just the host architecture is built.
sti::build::build_binaries() {
  # Create a sub-shell so that we don't pollute the outer environment
  (
    # Check for `go` binary and set ${GOPATH}.
    sti::build::setup_env

    # Fetch the version.
    local version_ldflags
    version_ldflags=$(sti::build::ldflags)

    sti::build::export_targets "$@"

    local platform
    for platform in "${platforms[@]}"; do
      sti::build::set_platform_envs "${platform}"
      echo "++ Building go targets for ${platform}:" "${targets[@]}"
      go install "${goflags[@]:+${goflags[@]}}" \
          -ldflags "${version_ldflags}" \
          "${binaries[@]}"
      sti::build::unset_platform_envs "${platform}"
    done
  )
}

# Generates the set of target packages, binaries, and platforms to build for.
# Accepts binaries via $@, and platforms via STI_BUILD_PLATFORMS, or defaults to
# the current platform.
sti::build::export_targets() {
  # Use eval to preserve embedded quoted strings.
  local goflags
  eval "goflags=(${STI_GOFLAGS:-})"

  targets=()
  local arg
  for arg; do
    if [[ "${arg}" == -* ]]; then
      # Assume arguments starting with a dash are flags to pass to go.
      goflags+=("${arg}")
    else
      targets+=("${arg}")
    fi
  done

  if [[ ${#targets[@]} -eq 0 ]]; then
    targets=("${STI_ALL_TARGETS[@]}")
  fi

  binaries=($(sti::build::binaries_from_targets "${targets[@]}"))

  platforms=("${STI_BUILD_PLATFORMS[@]:+${STI_BUILD_PLATFORMS[@]}}")
  if [[ ${#platforms[@]} -eq 0 ]]; then
    platforms=("$(sti::build::host_platform)")
  fi
}


# Takes the platform name ($1) and sets the appropriate golang env variables
# for that platform.
sti::build::set_platform_envs() {
  [[ -n ${1-} ]] || {
    echo "!!! Internal error.  No platform set in sti::build::set_platform_envs"
    exit 1
  }

  export GOOS=${platform%/*}
  export GOARCH=${platform##*/}
}

# Takes the platform name ($1) and resets the appropriate golang env variables
# for that platform.
sti::build::unset_platform_envs() {
  unset GOOS
  unset GOARCH
}


# Create the GOPATH tree under $STI_ROOT
sti::build::create_gopath_tree() {
  local go_pkg_dir="${STI_GOPATH}/src/${STI_GO_PACKAGE}"
  local go_pkg_basedir=$(dirname "${go_pkg_dir}")

  mkdir -p "${go_pkg_basedir}"
  rm -f "${go_pkg_dir}"

  # TODO: This symlink should be relative.
  ln -s "${STI_ROOT}" "${go_pkg_dir}"
}


# sti::build::setup_env will check that the `go` commands is available in
# ${PATH}. If not running on Travis, it will also check that the Go version is
# good enough for the Kubernetes build.
#
# Input Vars:
#   STI_EXTRA_GOPATH - If set, this is included in created GOPATH
#   STI_NO_GODEPS - If set, we don't add 'Godeps/_workspace' to GOPATH
#
# Output Vars:
#   export GOPATH - A modified GOPATH to our created tree along with extra
#     stuff.
#   export GOBIN - This is actively unset if already set as we want binaries
#     placed in a predictable place.
sti::build::setup_env() {
  sti::build::create_gopath_tree

  if [[ -z "$(which go)" ]]; then
    cat <<EOF

Can't find 'go' in PATH, please fix and retry.
See http://golang.org/doc/install for installation instructions.

EOF
    exit 2
  fi

  # Travis continuous build uses a head go release that doesn't report
  # a version number, so we skip this check on Travis.  It's unnecessary
  # there anyway.
  if [[ "${TRAVIS:-}" != "true" ]]; then
    local go_version
    go_version=($(go version))
    if [[ "${go_version[2]}" < "go1.4" ]]; then
      cat <<EOF

Detected go version: ${go_version[*]}.
S2I requires go version 1.4 or greater.
Please install Go version 1.4 or later.

EOF
      exit 2
    fi
  fi

  GOPATH=${STI_GOPATH}

  # Append STI_EXTRA_GOPATH to the GOPATH if it is defined.
  if [[ -n ${STI_EXTRA_GOPATH:-} ]]; then
    GOPATH="${GOPATH}:${STI_EXTRA_GOPATH}"
  fi

  # Append the tree maintained by `godep` to the GOPATH unless STI_NO_GODEPS
  # is defined.
  if [[ -z ${STI_NO_GODEPS:-} ]]; then
    GOPATH="${GOPATH}:${STI_ROOT}/Godeps/_workspace"
  fi
  export GOPATH

  # Unset GOBIN in case it already exists in the current session.
  unset GOBIN
}

# This will take binaries from $GOPATH/bin and copy them to the appropriate
# place in ${STI_OUTPUT_BINDIR}
#
# If STI_RELEASE_ARCHIVE is set to a directory, it will have tar archives of
# each STI_RELEASE_PLATFORMS created
#
# Ideally this wouldn't be necessary and we could just set GOBIN to
# STI_OUTPUT_BINDIR but that won't work in the face of cross compilation.  'go
# install' will place binaries that match the host platform directly in $GOBIN
# while placing cross compiled binaries into `platform_arch` subdirs.  This
# complicates pretty much everything else we do around packaging and such.
sti::build::place_bins() {
  (
    local host_platform
    host_platform=$(sti::build::host_platform)

    echo "++ Placing binaries"

    if [[ "${STI_RELEASE_ARCHIVE-}" != "" ]]; then
      sti::build::get_version_vars
      mkdir -p "${STI_LOCAL_RELEASEPATH}"
    fi

    sti::build::export_targets "$@"

    for platform in "${platforms[@]}"; do
      # The substitution on platform_src below will replace all slashes with
      # underscores.  It'll transform darwin/amd64 -> darwin_amd64.
      local platform_src="/${platform//\//_}"
      if [[ $platform == $host_platform ]]; then
        platform_src=""
      fi

      # Skip this directory if the platform has no binaries.
      local full_binpath_src="${STI_GOPATH}/bin${platform_src}"
      if [[ ! -d "${full_binpath_src}" ]]; then
        continue
      fi

      mkdir -p "${STI_OUTPUT_BINPATH}/${platform}"

      # Create an array of binaries to release. Append .exe variants if the platform is windows.
      local -a binaries=()
      for binary in "${targets[@]}"; do
        binary=$(basename $binary)
        if [[ $platform == "windows/amd64" ]]; then
          binaries+=("${binary}.exe")
        else
          binaries+=("${binary}")
        fi
      done

      # Move the specified release binaries to the shared STI_OUTPUT_BINPATH.
      for binary in "${binaries[@]}"; do
        mv "${full_binpath_src}/${binary}" "${STI_OUTPUT_BINPATH}/${platform}/"
      done

      # If no release archive was requested, we're done.
      if [[ "${STI_RELEASE_ARCHIVE-}" == "" ]]; then
        continue
      fi

      # Create a temporary bin directory containing only the binaries marked for release.
      local release_binpath=$(mktemp -d sti.release.${STI_RELEASE_ARCHIVE}.XXX)
      for binary in "${binaries[@]}"; do
        cp "${STI_OUTPUT_BINPATH}/${platform}/${binary}" "${release_binpath}/"
      done

      # Create binary copies where specified.
      local suffix=""
      if [[ $platform == "windows/amd64" ]]; then
        suffix=".exe"
      fi
      for linkname in "${STI_BINARY_COPY[@]}"; do
        local src="${release_binpath}/s2i${suffix}"
        if [[ -f "${src}" ]]; then
          cp "${release_binpath}/s2i${suffix}" "${release_binpath}/${linkname}${suffix}"
        fi
      done

      # Create the release archive.
      local platform_segment="${platform//\//-}"
      if [[ $platform == "windows/amd64" ]]; then
        local archive_name="${STI_RELEASE_ARCHIVE}-${STI_GIT_VERSION}-${STI_GIT_COMMIT}-${platform_segment}.zip"
        echo "++ Creating ${archive_name}"
        for file in "${STI_BINARY_RELEASE_WINDOWS[@]}"; do
          zip "${STI_LOCAL_RELEASEPATH}/${archive_name}" -qj "${release_binpath}/${file}"
        done
      else
        local archive_name="${STI_RELEASE_ARCHIVE}-${STI_GIT_VERSION}-${STI_GIT_COMMIT}-${platform_segment}.tar.gz"
        echo "++ Creating ${archive_name}"
        tar -czf "${STI_LOCAL_RELEASEPATH}/${archive_name}" -C "${release_binpath}" .
      fi
      rm -rf "${release_binpath}"
    done
  )
}

# sti::build::make_binary_symlinks makes symlinks for the sti
# binary in _output/local/go/bin
sti::build::make_binary_symlinks() {
  platform=$(sti::build::host_platform)
  if [[ -f "${STI_OUTPUT_BINPATH}/${platform}/s2i" ]]; then
    for linkname in "${STI_BINARY_SYMLINKS[@]}"; do
      if [[ $platform == "windows/amd64" ]]; then
        cp s2i "${STI_OUTPUT_BINPATH}/${platform}/${linkname}.exe"
      else
        ln -sf s2i "${STI_OUTPUT_BINPATH}/${platform}/${linkname}"
      fi
    done
  fi
}

# sti::build::detect_local_release_tars verifies there is only one primary and one
# image binaries release tar in STI_LOCAL_RELEASEPATH for the given platform specified by
# argument 1, exiting if more than one of either is found.
#
# If the tars are discovered, their full paths are exported to the following env vars:
#
#   STI_PRIMARY_RELEASE_TAR
sti::build::detect_local_release_tars() {
  local platform="$1"

  if [[ ! -d "${STI_LOCAL_RELEASEPATH}" ]]; then
    echo "There are no release artifacts in ${STI_LOCAL_RELEASEPATH}"
    exit 2
  fi
  if [[ ! -f "${STI_LOCAL_RELEASEPATH}/.commit" ]]; then
    echo "There is no release .commit identifier ${STI_LOCAL_RELEASEPATH}"
    exit 2
  fi
  local primary=$(find ${STI_LOCAL_RELEASEPATH} -maxdepth 1 -type f -name source-to-image-*-${platform}*)
  if [[ $(echo "${primary}" | wc -l) -ne 1 ]]; then
    echo "There should be exactly one ${platform} primary tar in $STI_LOCAL_RELEASEPATH"
    exit 2
  fi

  export STI_PRIMARY_RELEASE_TAR="${primary}"
  export STI_RELEASE_COMMIT="$(cat ${STI_LOCAL_RELEASEPATH}/.commit)"
}


# sti::build::get_version_vars loads the standard version variables as
# ENV vars
sti::build::get_version_vars() {
  if [[ -n ${STI_VERSION_FILE-} ]]; then
    source "${STI_VERSION_FILE}"
    return
  fi
  sti::build::sti_version_vars
}

# sti::build::sti_version_vars looks up the current Git vars
sti::build::sti_version_vars() {
  local git=(git --work-tree "${STI_ROOT}")

  if [[ -n ${STI_GIT_COMMIT-} ]] || STI_GIT_COMMIT=$("${git[@]}" rev-parse --short "HEAD^{commit}" 2>/dev/null); then
    if [[ -z ${STI_GIT_TREE_STATE-} ]]; then
      # Check if the tree is dirty.  default to dirty
      if git_status=$("${git[@]}" status --porcelain 2>/dev/null) && [[ -z ${git_status} ]]; then
        STI_GIT_TREE_STATE="clean"
      else
        STI_GIT_TREE_STATE="dirty"
      fi
    fi

    # Use git describe to find the version based on annotated tags.
    if [[ -n ${STI_GIT_VERSION-} ]] || STI_GIT_VERSION=$("${git[@]}" describe --tags "${STI_GIT_COMMIT}^{commit}" 2>/dev/null); then
      if [[ "${STI_GIT_TREE_STATE}" == "dirty" ]]; then
        # git describe --dirty only considers changes to existing files, but
        # that is problematic since new untracked .go files affect the build,
        # so use our idea of "dirty" from git status instead.
        STI_GIT_VERSION+="-dirty"
      fi

      # Try to match the "git describe" output to a regex to try to extract
      # the "major" and "minor" versions and whether this is the exact tagged
      # version or whether the tree is between two tagged versions.
      if [[ "${STI_GIT_VERSION}" =~ ^v([0-9]+)\.([0-9]+)([.-].*)?$ ]]; then
        STI_GIT_MAJOR=${BASH_REMATCH[1]}
        STI_GIT_MINOR=${BASH_REMATCH[2]}
        if [[ -n "${BASH_REMATCH[3]}" ]]; then
          STI_GIT_MINOR+="+"
        fi
      fi
    fi
  fi
}

# Saves the environment flags to $1
sti::build::save_version_vars() {
  local version_file=${1-}
  [[ -n ${version_file} ]] || {
    echo "!!! Internal error.  No file specified in sti::build::save_version_vars"
    return 1
  }

  cat <<EOF >"${version_file}"
STI_GIT_COMMIT='${STI_GIT_COMMIT-}'
STI_GIT_TREE_STATE='${STI_GIT_TREE_STATE-}'
STI_GIT_VERSION='${STI_GIT_VERSION-}'
STI_GIT_MAJOR='${STI_GIT_MAJOR-}'
STI_GIT_MINOR='${STI_GIT_MINOR-}'
EOF
}

# golang 1.5 wants `-X key=val`, but golang 1.4- REQUIRES `-X key val`
sti::build::ldflag() {
  local key=${1}
  local val=${2}

  GO_VERSION=($(go version))

  if [[ -z $(echo "${GO_VERSION[2]}" | grep -E 'go1.5') ]]; then
    echo "-X ${STI_GO_PACKAGE}/pkg/version.${key} ${val}"
  else
    echo "-X ${STI_GO_PACKAGE}/pkg/version.${key}=${val}"
  fi
}

# sti::build::ldflags calculates the -ldflags argument for building STI
sti::build::ldflags() {
  (
    # Run this in a subshell to prevent settings/variables from leaking.
    set -o errexit
    set -o nounset
    set -o pipefail

    cd "${STI_ROOT}"

    sti::build::get_version_vars

    declare -a ldflags=()
    ldflags+=($(sti::build::ldflag "majorFromGit" "${STI_GIT_MAJOR}"))
    ldflags+=($(sti::build::ldflag "minorFromGit" "${STI_GIT_MINOR}"))
    ldflags+=($(sti::build::ldflag "versionFromGit" "${STI_GIT_VERSION}"))
    ldflags+=($(sti::build::ldflag "commitFromGit" "${STI_GIT_COMMIT}"))

    # The -ldflags parameter takes a single string, so join the output.
    echo "${ldflags[*]-}"
  )
}

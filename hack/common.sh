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

readonly STI_GO_PACKAGE=github.com/openshift/source-to-image
readonly STI_GOPATH="${STI_OUTPUT}/go"

readonly STI_COMPILE_PLATFORMS=(
  linux/amd64
)
readonly STI_COMPILE_TARGETS=(
)

readonly STI_CROSS_COMPILE_PLATFORMS=(
  linux/amd64
  darwin/amd64
  windows/amd64
)
readonly STI_CROSS_COMPILE_TARGETS=(
  cmd/sti
)
# readonly STI_CROSS_COMPILE_BINARIES=("${STI_CROSS_COMPILE_TARGETS[@]##*/}")

readonly STI_ALL_TARGETS=(
  "${STI_COMPILE_TARGETS[@]-}"
  "${STI_CROSS_COMPILE_TARGETS[@]}"
)
# readonly STI_ALL_BINARIES=("${STI_ALL_TARGETS[@]##*/}")

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

    # Use eval to preserve embedded quoted strings.
    local goflags
    eval "goflags=(${STI_GOFLAGS:-})"

    local -a targets=()
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

    local -a platforms=("${STI_BUILD_PLATFORMS[@]:+${STI_BUILD_PLATFORMS[@]}}")
    if [[ ${#platforms[@]} -eq 0 ]]; then
      platforms=("$(sti::build::host_platform)")
    fi

    local binaries
    binaries=($(sti::build::binaries_from_targets "${targets[@]}"))

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
    echo <<EOF

Can't find 'go' in PATH, please fix and retry.
See http://golang.org/doc/install for installation instructions.

EOF
    exit 2
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
# If STI_RELEASE_ARCHIVES is set to a directory, it will have tar archives of
# each CROSS_COMPILE_PLATFORM created
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

    if [[ "${STI_RELEASE_ARCHIVES-}" != "" ]]; then
      sti::build::get_version_vars
      rm -rf "${STI_RELEASE_ARCHIVES}"
      mkdir -p "${STI_RELEASE_ARCHIVES}"
    fi

    local platform
    for platform in "${STI_CROSS_COMPILE_PLATFORMS[@]}"; do
      # The substitution on platform_src below will replace all slashes with
      # underscores.  It'll transform darwin/amd64 -> darwin_amd64.
      local platform_src="/${platform//\//_}"
      if [[ $platform == $host_platform ]]; then
        platform_src=""
      fi

      local full_binpath_src="${STI_GOPATH}/bin${platform_src}"
      if [[ -d "${full_binpath_src}" ]]; then
        mkdir -p "${STI_OUTPUT_BINPATH}/${platform}"
        find "${full_binpath_src}" -maxdepth 1 -type f -exec \
          rsync -pt {} "${STI_OUTPUT_BINPATH}/${platform}" \;

        if [[ "${STI_RELEASE_ARCHIVES-}" != "" ]]; then
          local platform_segment="${platform//\//-}"
          local archive_name="source-to-image-${STI_GIT_VERSION}-${STI_GIT_COMMIT}-${platform_segment}.tar.gz"
          echo "++ Creating ${archive_name}"
          tar -czf "${STI_RELEASE_ARCHIVES}/${archive_name}" -C "${STI_OUTPUT_BINPATH}/${platform}" .
        fi
      fi
    done
  )
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
    if [[ -n ${STI_GIT_VERSION-} ]] || STI_GIT_VERSION=$("${git[@]}" describe --abbrev=14 "${STI_GIT_COMMIT}^{commit}" 2>/dev/null); then
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
    ldflags+=(-X "${STI_GO_PACKAGE}/pkg/sti/version.commitFromGit" "${STI_GIT_COMMIT}")

    # The -ldflags parameter takes a single string, so join the output.
    echo "${ldflags[*]-}"
  )
}

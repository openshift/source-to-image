package main

import (
	"bytes"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	log "github.com/golang/glog"
	utilglog "github.com/openshift/source-to-image/pkg/util/glog"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/api/describe"
	"github.com/openshift/source-to-image/pkg/api/validation"
	"github.com/openshift/source-to-image/pkg/build"
	"github.com/openshift/source-to-image/pkg/build/strategies"
	"github.com/openshift/source-to-image/pkg/build/strategies/sti"
	cmdutil "github.com/openshift/source-to-image/pkg/cmd"
	"github.com/openshift/source-to-image/pkg/config"
	"github.com/openshift/source-to-image/pkg/create"
	"github.com/openshift/source-to-image/pkg/docker"
	s2ierr "github.com/openshift/source-to-image/pkg/errors"
	"github.com/openshift/source-to-image/pkg/run"
	"github.com/openshift/source-to-image/pkg/tar"
	"github.com/openshift/source-to-image/pkg/util"
	"github.com/openshift/source-to-image/pkg/version"
	"io"
)

// glog is a placeholder until the builders pass an output stream down
// client facing libraries should not be using glog
var glog = utilglog.StderrLog

func newCmdVersion() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Display version",
		Long:  "Display version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("s2i %v\n", version.Get())
		},
	}
}

func newCmdBuild(cfg *api.Config) *cobra.Command {
	useConfig := false
	oldScriptsFlag := ""
	oldDestination := ""

	buildCmd := &cobra.Command{
		Use:   "build <source> <image> [<tag>]",
		Short: "Build a new image",
		Long:  "Build a new Docker image named <tag> (if provided) from a source repository and base image.",
		Example: `
# Build an application Docker image from a Git repository
$ s2i build git://github.com/openshift/ruby-hello-world centos/ruby-22-centos7 hello-world-app

# Build from a local directory
$ s2i build . centos/ruby-22-centos7 hello-world-app
`,
		Run: func(cmd *cobra.Command, args []string) {
			glog.V(1).Infof("Running S2I version %q\n", version.Get())

			// Attempt to restore the build command from the configuration file
			if useConfig {
				config.Restore(cfg, cmd)
			}

			// If user specifies the arguments, then we override the stored ones
			if len(args) >= 2 {
				cfg.Source = args[0]
				cfg.BuilderImage = args[1]
				if len(args) >= 3 {
					cfg.Tag = args[2]
				}
			}

			if cfg.Incremental && len(cfg.RuntimeImage) > 0 {
				fmt.Fprintln(os.Stderr, "ERROR: Incremental build with runtime image isn't supported")
				os.Exit(1)
			}
			if cfg.ForcePull {
				glog.Warning("DEPRECATED: The '--force-pull' option is deprecated. Use '--pull-policy' instead")
			}

			if len(cfg.BuilderPullPolicy) == 0 {
				cfg.BuilderPullPolicy = api.DefaultBuilderPullPolicy
			}
			if len(cfg.PreviousImagePullPolicy) == 0 {
				cfg.PreviousImagePullPolicy = api.DefaultPreviousImagePullPolicy
			}

			if errs := validation.ValidateConfig(cfg); len(errs) > 0 {
				for _, e := range errs {
					fmt.Fprintf(os.Stderr, "ERROR: %s\n", e)
				}
				fmt.Println()
				cmd.Help()
				os.Exit(1)
			}

			// Persists the current command line options and config into .s2ifile
			if useConfig {
				config.Save(cfg, cmd)
			}

			// Attempt to read the .dockercfg and extract the authentication for
			// docker pull
			if r, err := os.Open(cfg.DockerCfgPath); err == nil {
				defer r.Close()
				auths := docker.LoadImageRegistryAuth(r)
				cfg.PullAuthentication = docker.GetImageRegistryAuth(auths, cfg.BuilderImage)
				if cfg.Incremental {
					cfg.IncrementalAuthentication = docker.GetImageRegistryAuth(auths, cfg.Tag)
				}
				if len(cfg.RuntimeImage) > 0 {
					cfg.RuntimeAuthentication = docker.GetImageRegistryAuth(auths, cfg.RuntimeImage)
				}
			}

			if len(cfg.EnvironmentFile) > 0 {
				result, err := util.ReadEnvironmentFile(cfg.EnvironmentFile)
				if err != nil {
					glog.Warningf("Unable to read environment file %q: %v", cfg.EnvironmentFile, err)
				} else {
					for name, value := range result {
						cfg.Environment = append(cfg.Environment, api.EnvironmentSpec{Name: name, Value: value})
					}
				}
			}

			if len(oldScriptsFlag) != 0 {
				glog.Warning("DEPRECATED: Flag --scripts is deprecated, use --scripts-url instead")
				cfg.ScriptsURL = oldScriptsFlag
			}
			if len(oldDestination) != 0 {
				glog.Warning("DEPRECATED: Flag --location is deprecated, use --destination instead")
				cfg.Destination = oldDestination
			}

			glog.V(2).Infof("\n%s\n", describe.Config(cfg))

			err := docker.CheckReachable(cfg)
			if err != nil {
				glog.Fatal(err)
			}

			builder, _, err := strategies.GetStrategy(cfg)
			s2ierr.CheckError(err)
			result, err := builder.Build(cfg)
			s2ierr.CheckError(err)

			for _, message := range result.Messages {
				glog.V(1).Infof(message)
			}

			if cfg.RunImage {
				runner, err := run.New(cfg)
				s2ierr.CheckError(err)
				err = runner.Run(cfg)
				s2ierr.CheckError(err)
			}
		},
	}

	cmdutil.AddCommonFlags(buildCmd, cfg)

	buildCmd.Flags().BoolVar(&(cfg.RunImage), "run", false, "Run resulting image as part of invocation of this command")
	buildCmd.Flags().BoolVar(&(cfg.IgnoreSubmodules), "ignore-submodules", false, "Ignore all git submodules when cloning application repository")
	buildCmd.Flags().VarP(&(cfg.Environment), "env", "e", "Specify an single environment variable in NAME=VALUE format")
	buildCmd.Flags().StringVarP(&(cfg.Ref), "ref", "r", "", "Specify a ref to check-out")
	buildCmd.Flags().StringVarP(&(cfg.AssembleUser), "assemble-user", "", "", "Specify the user to run assemble with")
	buildCmd.Flags().StringVarP(&(cfg.ContextDir), "context-dir", "", "", "Specify the sub-directory inside the repository with the application sources")
	buildCmd.Flags().StringVarP(&(cfg.ExcludeRegExp), "exclude", "", tar.DefaultExclusionPattern.String(), "Regular expression for selecting files from the source tree to exclude from the build, where the default excludes the '.git' directory (see https://golang.org/pkg/regexp for syntax, but note that \"\" will be interpreted as allow all files and exclude no files)")
	buildCmd.Flags().StringVarP(&(cfg.ScriptsURL), "scripts-url", "s", "", "Specify a URL for the assemble, assemble-runtime and run scripts")
	buildCmd.Flags().StringVar(&(oldScriptsFlag), "scripts", "", "DEPRECATED: Specify a URL for the assemble and run scripts")
	buildCmd.Flags().BoolVar(&(useConfig), "use-config", false, "Store command line options to .s2ifile")
	buildCmd.Flags().StringVarP(&(cfg.EnvironmentFile), "environment-file", "E", "", "Specify the path to the file with environment")
	buildCmd.Flags().StringVarP(&(cfg.DisplayName), "application-name", "n", "", "Specify the display name for the application (default: output image name)")
	buildCmd.Flags().StringVarP(&(cfg.Description), "description", "", "", "Specify the description of the application")
	buildCmd.Flags().VarP(&(cfg.AllowedUIDs), "allowed-uids", "u", "Specify a range of allowed user ids for the builder and runtime images")
	buildCmd.Flags().VarP(&(cfg.Injections), "inject", "i", "Specify a directory to inject into the assemble container")
	buildCmd.Flags().VarP(&(cfg.BuildVolumes), "volume", "v", "Specify a volume to mount into the assemble container")
	buildCmd.Flags().StringSliceVar(&(cfg.DropCapabilities), "cap-drop", []string{}, "Specify a comma-separated list of capabilities to drop when running Docker containers")
	buildCmd.Flags().StringVarP(&(oldDestination), "location", "l", "",
		"DEPRECATED: Specify a destination location for untar operation")
	buildCmd.Flags().BoolVarP(&(cfg.ForceCopy), "copy", "c", false, "Use local file system copy instead of git cloning the source url")
	buildCmd.Flags().StringVar(&(cfg.RuntimeImage), "runtime-image", "", "Image that will be used as the base for the runtime image")
	buildCmd.Flags().VarP(&(cfg.RuntimeArtifacts), "runtime-artifact", "a", "Specify a file or directory to be copied from the builder to the runtime image")

	return buildCmd
}

func newCmdRebuild(cfg *api.Config) *cobra.Command {
	buildCmd := &cobra.Command{
		Use:   "rebuild <image> [<new-tag>]",
		Short: "Rebuild an existing image",
		Long:  "Rebuild an existing application image that was built by S2I previously.",
		Run: func(cmd *cobra.Command, args []string) {
			// If user specifies the arguments, then we override the stored ones
			if len(args) >= 1 {
				cfg.Tag = args[0]
			} else {
				cmd.Help()
				os.Exit(1)
			}

			var auths *docker.AuthConfigurations
			r, err := os.Open(cfg.DockerCfgPath)
			if err == nil {
				defer r.Close()
				auths = docker.LoadImageRegistryAuth(r)
			}

			cfg.PullAuthentication = docker.GetImageRegistryAuth(auths, cfg.Tag)

			pr, err := docker.GetRebuildImage(cfg)
			s2ierr.CheckError(err)
			err = build.GenerateConfigFromLabels(cfg, pr)
			s2ierr.CheckError(err)

			if len(args) >= 2 {
				cfg.Tag = args[1]
			}

			cfg.PullAuthentication = docker.GetImageRegistryAuth(auths, cfg.BuilderImage)

			if len(cfg.BuilderPullPolicy) == 0 {
				cfg.BuilderPullPolicy = api.DefaultBuilderPullPolicy
			}
			if len(cfg.PreviousImagePullPolicy) == 0 {
				cfg.PreviousImagePullPolicy = api.DefaultPreviousImagePullPolicy
			}

			glog.V(2).Infof("\n%s\n", describe.Config(cfg))

			builder, _, err := strategies.GetStrategy(cfg)
			s2ierr.CheckError(err)
			result, err := builder.Build(cfg)
			s2ierr.CheckError(err)

			for _, message := range result.Messages {
				glog.V(1).Infof(message)
			}
		},
	}

	cmdutil.AddCommonFlags(buildCmd, cfg)
	return buildCmd
}

func newCmdCreate() *cobra.Command {
	return &cobra.Command{
		Use:   "create <imageName> <destination>",
		Short: "Bootstrap a new S2I image repository",
		Long:  "Bootstrap a new S2I image with given imageName inside the destination directory",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) < 2 {
				cmd.Help()
				os.Exit(1)
			}
			b := create.New(args[0], args[1])
			b.AddSTIScripts()
			b.AddDockerfile()
			b.AddTests()
		},
	}
}

var (
	completionShells = map[string]func(out io.Writer, cmd *cobra.Command) error{
		"bash": runCompletionBash,
		"zsh":  runCompletionZsh,
	}
)

func newCmdCompletion(root *cobra.Command) *cobra.Command {
	shells := []string{}
	for s := range completionShells {
		shells = append(shells, s)
	}

	return &cobra.Command{
		Use:   "completion SHELL",
		Short: "Generate completion for the s2i command (bash or zsh)",
		Long:  "Generate completion for the s2i command into standard output  (bash or zsh)",
		Run: func(cmd *cobra.Command, args []string) {
			// TODO: The version of cobra we vendor takes a
			// *bytes.Buffer, while newer versions take an
			// io.Writer. The code below could be simplified to a
			// single line `root.GenBashCompletion(os.Stdout)` when
			// we update cobra.
			var out bytes.Buffer
			err := RunCompletion(&out, cmd, root, args)
			if err != nil {
				s2ierr.CheckError(err)
			} else {
				fmt.Print(out.String())
			}
		},
		ValidArgs: shells,
	}
}

// RunCompletion first checks args[0] to decide compose zsh or bash
// then write the content into the out bytes.Buffer
// if command input error will call UsageError with `cmd cobra.Command`
// `root cobra.Command` mainly for GenBashCompletion function
func RunCompletion(out io.Writer, cmd *cobra.Command, root *cobra.Command, args []string) error {
	var msg string
	if len(args) == 0 {
		msg = fmt.Sprintf("shell not specified.\nSee '%s -h' for help and examples.", cmd.CommandPath())
		return s2ierr.UsageError(msg)
	}
	if len(args) > 1 {
		msg = fmt.Sprintf("too many arguments. Expected only the shell type.\nSee '%s -h' for help and examples.", cmd.CommandPath())
		return s2ierr.UsageError(msg)
	}
	run, found := completionShells[args[0]]
	if !found {
		msg = fmt.Sprintf("unsupported shell type %q.\nSee '%s -h' for help and examples.", args[0], cmd.CommandPath())
		return s2ierr.UsageError(msg)
	}

	return run(out, root)
}

func runCompletionBash(out io.Writer, root *cobra.Command) error {
	return root.GenBashCompletion(out)
}

func runCompletionZsh(out io.Writer, root *cobra.Command) error {
	zshInitialization := `# Copyright 2016 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

__kubectl_bash_source() {
	alias shopt=':'
	alias _expand=_bash_expand
	alias _complete=_bash_comp
	emulate -L sh
	setopt kshglob noshglob braceexpand

	source "$@"
}

__kubectl_type() {
	# -t is not supported by zsh
	if [ "$1" == "-t" ]; then
		shift

		# fake Bash 4 to disable "complete -o nospace". Instead
		# "compopt +-o nospace" is used in the code to toggle trailing
		# spaces. We don't support that, but leave trailing spaces on
		# all the time
		if [ "$1" = "__kubectl_compopt" ]; then
			echo builtin
			return 0
		fi
	fi
	type "$@"
}

__kubectl_compgen() {
	local completions w
	completions=( $(compgen "$@") ) || return $?

	# filter by given word as prefix
	while [[ "$1" = -* && "$1" != -- ]]; do
		shift
		shift
	done
	if [[ "$1" == -- ]]; then
		shift
	fi
	for w in "${completions[@]}"; do
		if [[ "${w}" = "$1"* ]]; then
			echo "${w}"
		fi
	done
}

__kubectl_compopt() {
	true # don't do anything. Not supported by bashcompinit in zsh
}

__kubectl_declare() {
	if [ "$1" == "-F" ]; then
		whence -w "$@"
	else
		builtin declare "$@"
	fi
}

__kubectl_ltrim_colon_completions()
{
	if [[ "$1" == *:* && "$COMP_WORDBREAKS" == *:* ]]; then
		# Remove colon-word prefix from COMPREPLY items
		local colon_word=${1%${1##*:}}
		local i=${#COMPREPLY[*]}
		while [[ $((--i)) -ge 0 ]]; do
			COMPREPLY[$i]=${COMPREPLY[$i]#"$colon_word"}
		done
	fi
}

__kubectl_get_comp_words_by_ref() {
	cur="${COMP_WORDS[COMP_CWORD]}"
	prev="${COMP_WORDS[${COMP_CWORD}-1]}"
	words=("${COMP_WORDS[@]}")
	cword=("${COMP_CWORD[@]}")
}

__kubectl_filedir() {
	local RET OLD_IFS w qw

	__debug "_filedir $@ cur=$cur"
	if [[ "$1" = \~* ]]; then
		# somehow does not work. Maybe, zsh does not call this at all
		eval echo "$1"
		return 0
	fi

	OLD_IFS="$IFS"
	IFS=$'\n'
	if [ "$1" = "-d" ]; then
		shift
		RET=( $(compgen -d) )
	else
		RET=( $(compgen -f) )
	fi
	IFS="$OLD_IFS"

	IFS="," __debug "RET=${RET[@]} len=${#RET[@]}"

	for w in ${RET[@]}; do
		if [[ ! "${w}" = "${cur}"* ]]; then
			continue
		fi
		if eval "[[ \"\${w}\" = *.$1 || -d \"\${w}\" ]]"; then
			qw="$(__kubectl_quote "${w}")"
			if [ -d "${w}" ]; then
				COMPREPLY+=("${qw}/")
			else
				COMPREPLY+=("${qw}")
			fi
		fi
	done
}

__kubectl_quote() {
    if [[ $1 == \'* || $1 == \"* ]]; then
        # Leave out first character
        printf %q "${1:1}"
    else
    	printf %q "$1"
    fi
}

autoload -U +X bashcompinit && bashcompinit

# use word boundary patterns for BSD or GNU sed
LWORD='[[:<:]]'
RWORD='[[:>:]]'
if sed --help 2>&1 | grep -q GNU; then
	LWORD='\<'
	RWORD='\>'
fi

__kubectl_convert_bash_to_zsh() {
	sed \
	-e 's/declare -F/whence -w/' \
	-e 's/local \([a-zA-Z0-9_]*\)=/local \1; \1=/' \
	-e 's/flags+=("\(--.*\)=")/flags+=("\1"); two_word_flags+=("\1")/' \
	-e 's/must_have_one_flag+=("\(--.*\)=")/must_have_one_flag+=("\1")/' \
	-e "s/${LWORD}_filedir${RWORD}/__kubectl_filedir/g" \
	-e "s/${LWORD}_get_comp_words_by_ref${RWORD}/__kubectl_get_comp_words_by_ref/g" \
	-e "s/${LWORD}__ltrim_colon_completions${RWORD}/__kubectl_ltrim_colon_completions/g" \
	-e "s/${LWORD}compgen${RWORD}/__kubectl_compgen/g" \
	-e "s/${LWORD}compopt${RWORD}/__kubectl_compopt/g" \
	-e "s/${LWORD}declare${RWORD}/__kubectl_declare/g" \
	-e "s/\\\$(type${RWORD}/\$(__kubectl_type/g" \
	<<'BASH_COMPLETION_EOF'
`
	out.Write([]byte(zshInitialization))

	buf := new(bytes.Buffer)
	root.GenBashCompletion(buf)
	out.Write(buf.Bytes())

	zshTail := `
BASH_COMPLETION_EOF
}

__kubectl_bash_source <(__kubectl_convert_bash_to_zsh)
`
	out.Write([]byte(zshTail))
	return nil
}

func newCmdUsage(cfg *api.Config) *cobra.Command {
	oldScriptsFlag := ""
	oldDestination := ""

	usageCmd := &cobra.Command{
		Use:   "usage <image>",
		Short: "Print usage of the assemble script associated with the image",
		Long:  "Create and start a container from the image and invoke its usage script.",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				cmd.Help()
				os.Exit(1)
			}

			cfg.Usage = true
			cfg.BuilderImage = args[0]

			if len(oldScriptsFlag) != 0 {
				glog.Warning("DEPRECATED: Flag --scripts is deprecated, use --scripts-url instead")
				cfg.ScriptsURL = oldScriptsFlag
			}

			if len(cfg.BuilderPullPolicy) == 0 {
				cfg.BuilderPullPolicy = api.DefaultBuilderPullPolicy
			}
			if len(cfg.PreviousImagePullPolicy) == 0 {
				cfg.PreviousImagePullPolicy = api.DefaultPreviousImagePullPolicy
			}

			uh, err := sti.NewUsage(cfg)
			s2ierr.CheckError(err)
			err = uh.Show()
			s2ierr.CheckError(err)
		},
	}
	usageCmd.Flags().StringVarP(&(oldDestination), "location", "l", "",
		"Specify a destination location for untar operation")
	cmdutil.AddCommonFlags(usageCmd, cfg)
	return usageCmd
}

// setupGlog makes --loglevel reflect in glog's -v flag
func setupGlog(flags *pflag.FlagSet) {

	from := flag.CommandLine
	if fflag := from.Lookup("v"); fflag != nil {
		level := fflag.Value.(*log.Level)
		loglevelPtr := (*int32)(level)
		flags.Int32Var(loglevelPtr, "loglevel", 0, "Set the level of log output (0-5)")
	}

	// FIXME currently glog has only option to redirect output to stderr
	// the preferred for S2I would be to redirect to stdout
	flag.CommandLine.Set("logtostderr", "true")
}

func main() {
	rand.Seed(time.Now().UnixNano())

	// Applying partial glog flag initialization workaround from: https://github.com/kubernetes/kubernetes/issues/17162
	// Without this fake command line parse, glog will compain its flags have not been interpreted
	flag.CommandLine.Parse([]string{})

	cfg := &api.Config{}
	s2iCmd := &cobra.Command{
		Use: "s2i",
		Long: "Source-to-image (S2I) is a tool for building repeatable docker images.\n\n" +
			"A command line interface that injects and assembles source code into a docker image.\n" +
			"Complete documentation is available at http://github.com/openshift/source-to-image",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}
	cfg.DockerConfig = docker.GetDefaultDockerConfig()
	s2iCmd.PersistentFlags().StringVarP(&(cfg.DockerConfig.Endpoint), "url", "U", cfg.DockerConfig.Endpoint, "Set the url of the docker socket to use")
	s2iCmd.PersistentFlags().StringVar(&(cfg.DockerConfig.CertFile), "cert", cfg.DockerConfig.CertFile, "Set the path of the docker TLS certificate file")
	s2iCmd.PersistentFlags().StringVar(&(cfg.DockerConfig.KeyFile), "key", cfg.DockerConfig.KeyFile, "Set the path of the docker TLS key file")
	s2iCmd.PersistentFlags().StringVar(&(cfg.DockerConfig.CAFile), "ca", cfg.DockerConfig.CAFile, "Set the path of the docker TLS ca file")
	s2iCmd.PersistentFlags().BoolVar(&(cfg.DockerConfig.UseTLS), "tls", cfg.DockerConfig.UseTLS, "Use TLS to connect to docker; implied by --tlsverify")
	s2iCmd.PersistentFlags().BoolVar(&(cfg.DockerConfig.TLSVerify), "tlsverify", cfg.DockerConfig.TLSVerify, "Use TLS to connect to docker and verify the remote")
	s2iCmd.AddCommand(newCmdVersion())
	s2iCmd.AddCommand(newCmdBuild(cfg))
	s2iCmd.AddCommand(newCmdRebuild(cfg))
	s2iCmd.AddCommand(newCmdUsage(cfg))
	s2iCmd.AddCommand(newCmdCreate())
	setupGlog(s2iCmd.PersistentFlags())
	basename := filepath.Base(os.Args[0])
	// Make case-insensitive and strip executable suffix if present
	if runtime.GOOS == "windows" {
		basename = strings.ToLower(basename)
		basename = strings.TrimSuffix(basename, ".exe")
	}
	if basename == "sti" {
		glog.Warning("sti binary is deprecated, use s2i instead")
	}

	s2iCmd.AddCommand(newCmdCompletion(s2iCmd))

	err := s2iCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

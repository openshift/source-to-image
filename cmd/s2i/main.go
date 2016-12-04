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
	"github.com/openshift/source-to-image/pkg/errors"
	"github.com/openshift/source-to-image/pkg/run"
	"github.com/openshift/source-to-image/pkg/tar"
	"github.com/openshift/source-to-image/pkg/util"
	"github.com/openshift/source-to-image/pkg/version"
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
# Build a Docker image from a remote Git repository
$ s2i build https://github.com/openshift/ruby-hello-world centos/ruby-22-centos7 hello-world-app

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
			checkErr(err)
			result, err := builder.Build(cfg)
			checkErr(err)

			for _, message := range result.Messages {
				glog.V(1).Infof(message)
			}

			if cfg.RunImage {
				runner, err := run.New(cfg)
				checkErr(err)
				err = runner.Run(cfg)
				checkErr(err)
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
			checkErr(err)
			err = build.GenerateConfigFromLabels(cfg, pr)
			checkErr(err)

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
			checkErr(err)
			result, err := builder.Build(cfg)
			checkErr(err)

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

func newCmdGenBashCompletion(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   "genbashcompletion",
		Short: "Generate Bash completion for the s2i command",
		Long:  "Generate Bash completion for the s2i command into standard output",
		Run: func(cmd *cobra.Command, args []string) {
			// TODO: The version of cobra we vendor takes a
			// *bytes.Buffer, while newer versions take an
			// io.Writer. The code below could be simplified to a
			// single line `root.GenBashCompletion(os.Stdout)` when
			// we update cobra.
			var out bytes.Buffer
			root.GenBashCompletion(&out)
			fmt.Print(out.String())
		},
	}
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
			checkErr(err)
			err = uh.Show()
			checkErr(err)
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

func checkErr(err error) {
	if err == nil {
		return
	}
	if e, ok := err.(errors.Error); ok {
		glog.Errorf("An error occurred: %v", e)
		glog.Errorf("Suggested solution: %v", e.Suggestion)
		if e.Details != nil {
			glog.V(1).Infof("Details: %v", e.Details)
		}
		glog.Error("If the problem persists consult the docs at https://github.com/openshift/source-to-image/tree/master/docs. " +
			"Eventually reach us on freenode #openshift or file an issue at https://github.com/openshift/source-to-image/issues " +
			"providing us with a log from your build using --loglevel=3")
		os.Exit(e.ErrorCode)
	} else {
		glog.Errorf("An error occurred: %v", err)
		os.Exit(1)
	}
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

	s2iCmd.AddCommand(newCmdGenBashCompletion(s2iCmd))

	err := s2iCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/golang/glog"
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
	"github.com/openshift/source-to-image/pkg/util"
	"github.com/openshift/source-to-image/pkg/version"
)

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
			if glog.V(1) {
				glog.Infof("Running S2I version %q\n", version.Get())
			}

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
			}

			cfg.Environment = map[string]string{}
			if len(cfg.EnvironmentFile) > 0 {
				result, err := util.ReadEnvironmentFile(cfg.EnvironmentFile)
				if err != nil {
					glog.Warningf("Unable to read environment file %q: %v", cfg.EnvironmentFile, err)
				} else {
					cfg.Environment = result
				}
			}

			envs, err := cmdutil.ParseEnvs(cmd, "env")
			checkErr(err)
			for k, v := range envs {
				cfg.Environment[k] = v
			}

			if len(oldScriptsFlag) != 0 {
				glog.Warning("DEPRECATED: Flag --scripts is deprecated, use --scripts-url instead")
				cfg.ScriptsURL = oldScriptsFlag
			}
			if len(oldDestination) != 0 {
				glog.Warning("DEPRECATED: Flag --location is deprecated, use --destination instead")
				cfg.Destination = oldDestination
			}

			if glog.V(2) {
				fmt.Printf("\n%s\n", describe.DescribeConfig(cfg))
			}

			if !docker.IsReachable(cfg) {
				glog.Fatalf("Unable to connect to Docker daemon. Please set the DOCKER_HOST or make sure the Docker socket %q exists", cfg.DockerConfig.Endpoint)
			}

			builder, err := strategies.GetStrategy(cfg)
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
	buildCmd.Flags().BoolVar(&(cfg.DisableRecursive), "recursive", true, "Fetch all git submodules when cloning application repository")
	buildCmd.Flags().StringP("env", "e", "", "Specify an environment var NAME=VALUE,NAME2=VALUE2,...")
	buildCmd.Flags().StringVarP(&(cfg.Ref), "ref", "r", "", "Specify a ref to check-out")
	buildCmd.Flags().StringVarP(&(cfg.AssembleUser), "assemble-user", "", "", "Specify the user to run assemble with")
	buildCmd.Flags().StringVarP(&(cfg.ContextDir), "context-dir", "", "", "Specify the sub-directory inside the repository with the application sources")
	buildCmd.Flags().StringVarP(&(cfg.ScriptsURL), "scripts-url", "s", "", "Specify a URL for the assemble and run scripts")
	buildCmd.Flags().StringVar(&(oldScriptsFlag), "scripts", "", "DEPRECATED: Specify a URL for the assemble and run scripts")
	buildCmd.Flags().BoolVar(&(useConfig), "use-config", false, "Store command line options to .stifile")
	buildCmd.Flags().StringVarP(&(cfg.EnvironmentFile), "environment-file", "E", "", "Specify the path to the file with environment")
	buildCmd.Flags().StringVarP(&(cfg.DisplayName), "application-name", "n", "", "Specify the display name for the application (default: output image name)")
	buildCmd.Flags().StringVarP(&(cfg.Description), "description", "", "", "Specify the description of the application")
	buildCmd.Flags().VarP(&(cfg.AllowedUIDs), "allowed-uids", "u", "Specify a range of allowed user ids for the builder image")
	buildCmd.Flags().VarP(&(cfg.Injections), "inject", "i", "Specify a list of directories to inject into the assemble container")
	buildCmd.Flags().StringVarP(&(oldDestination), "location", "l", "",
		"DEPRECATED: Specify a destination location for untar operation")

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

			if r, err := os.Open(cfg.DockerCfgPath); err == nil {
				cfg.PullAuthentication = docker.LoadAndGetImageRegistryAuth(r, cfg.Tag)
			}

			err := build.GenerateConfigFromLabels(cfg.Tag, cfg)
			checkErr(err)

			if len(args) >= 2 {
				cfg.Tag = args[1]
			}

			// Attempt to read the .dockercfg and extract the authentication for
			// docker pull
			if r, err := os.Open(cfg.DockerCfgPath); err == nil {
				cfg.PullAuthentication = docker.LoadAndGetImageRegistryAuth(r, cfg.BuilderImage)
			}

			if len(cfg.BuilderPullPolicy) == 0 {
				cfg.BuilderPullPolicy = api.DefaultBuilderPullPolicy
			}
			if len(cfg.PreviousImagePullPolicy) == 0 {
				cfg.PreviousImagePullPolicy = api.DefaultPreviousImagePullPolicy
			}

			if glog.V(2) {
				fmt.Printf("\n%s\n", describe.DescribeConfig(cfg))
			}

			builder, err := strategies.GetStrategy(cfg)
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
			envs, err := cmdutil.ParseEnvs(cmd, "env")
			checkErr(err)
			cfg.Environment = envs

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

func setupGlog(flags *pflag.FlagSet) {
	from := flag.CommandLine
	if fflag := from.Lookup("v"); fflag != nil {
		level := fflag.Value.(*glog.Level)
		levelPtr := (*int32)(level)
		flags.Int32Var(levelPtr, "loglevel", 0, "Set the level of log output (0-5)")
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
		glog.V(1).Infof("An error occurred: %v", err)
		os.Exit(1)
	}
}

func main() {
	cfg := &api.Config{}
	stiCmd := &cobra.Command{
		Use: "s2i",
		Long: "Source-to-image (S2I) is a tool for building repeatable docker images.\n\n" +
			"A command line interface that injects and assembles source code into a docker image.\n" +
			"Complete documentation is available at http://github.com/openshift/source-to-image",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}
	cfg.DockerConfig = docker.GetDefaultDockerConfig()
	stiCmd.PersistentFlags().StringVarP(&(cfg.DockerConfig.Endpoint), "url", "U", cfg.DockerConfig.Endpoint, "Set the url of the docker socket to use")
	stiCmd.PersistentFlags().StringVar(&(cfg.DockerConfig.CertFile), "cert", cfg.DockerConfig.CertFile, "Set the path of the docker TLS certificate file")
	stiCmd.PersistentFlags().StringVar(&(cfg.DockerConfig.KeyFile), "key", cfg.DockerConfig.KeyFile, "Set the path of the docker TLS key file")
	stiCmd.PersistentFlags().StringVar(&(cfg.DockerConfig.CAFile), "ca", cfg.DockerConfig.CAFile, "Set the path of the docker TLS ca file")

	stiCmd.AddCommand(newCmdVersion())
	stiCmd.AddCommand(newCmdBuild(cfg))
	stiCmd.AddCommand(newCmdRebuild(cfg))
	stiCmd.AddCommand(newCmdUsage(cfg))
	stiCmd.AddCommand(newCmdCreate())
	setupGlog(stiCmd.PersistentFlags())

	basename := filepath.Base(os.Args[0])
	// Make case-insensitive and strip executable suffix if present
	if runtime.GOOS == "windows" {
		basename = strings.ToLower(basename)
		basename = strings.TrimSuffix(basename, ".exe")
	}
	if basename == "sti" {
		glog.Warning("sti binary is deprecated, use s2i instead")
	}

	err := stiCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

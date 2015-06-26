package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/api/describe"
	"github.com/openshift/source-to-image/pkg/api/validation"
	"github.com/openshift/source-to-image/pkg/build/strategies"
	"github.com/openshift/source-to-image/pkg/build/strategies/sti"
	"github.com/openshift/source-to-image/pkg/config"
	"github.com/openshift/source-to-image/pkg/create"
	"github.com/openshift/source-to-image/pkg/docker"
	"github.com/openshift/source-to-image/pkg/errors"
	"github.com/openshift/source-to-image/pkg/util"
	"github.com/openshift/source-to-image/pkg/version"
)

func parseEnvs(cmd *cobra.Command, name string) (map[string]string, error) {
	env := cmd.Flags().Lookup(name)
	if env == nil || len(env.Value.String()) == 0 {
		return nil, nil
	}

	envs := make(map[string]string)
	pairs := strings.Split(env.Value.String(), ",")
	for _, pair := range pairs {
		atoms := strings.Split(pair, "=")
		if len(atoms) != 2 {
			return nil, fmt.Errorf("malformed env string: %s", pair)
		}
		envs[atoms[0]] = atoms[1]
	}

	return envs, nil
}

func defaultDockerConfig() *api.DockerConfig {
	cfg := &api.DockerConfig{}
	if cfg.Endpoint = os.Getenv("DOCKER_HOST"); cfg.Endpoint == "" {
		cfg.Endpoint = "unix:///var/run/docker.sock"
	}
	if os.Getenv("DOCKER_TLS_VERIFY") == "1" {
		certPath := os.Getenv("DOCKER_CERT_PATH")
		cfg.CertFile = filepath.Join(certPath, "cert.pem")
		cfg.KeyFile = filepath.Join(certPath, "key.pem")
		cfg.CAFile = filepath.Join(certPath, "ca.pem")
	}
	return cfg
}

func newCmdVersion() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Display version",
		Long:  "Display version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("sti %v\n", version.Get())
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
		Run: func(cmd *cobra.Command, args []string) {
			go func() {
				for {
					sigs := make(chan os.Signal, 1)
					signal.Notify(sigs, syscall.SIGQUIT)
					buf := make([]byte, 1<<20)
					for {
						<-sigs
						runtime.Stack(buf, true)
						if file, err := ioutil.TempFile(os.TempDir(), "sti_dump"); err == nil {
							defer file.Close()
							file.Write(buf)
						}
						glog.Infof("=== received SIGQUIT ===\n*** goroutine dump...\n%s\n*** end\n", buf)
					}
				}
			}()
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

			if len(validation.ValidateConfig(cfg)) != 0 {
				cmd.Help()
				os.Exit(1)
			}

			// Persists the current command line options and config into .stifile
			if useConfig {
				config.Save(cfg, cmd)
			}

			// Attempt to read the .dockercfg and extract the authentication for
			// docker pull
			if r, err := os.Open(cfg.DockerCfgPath); err == nil {
				cfg.PullAuthentication = docker.GetImageRegistryAuth(r, cfg.BuilderImage)
			}

			cfg.Environment = map[string]string{}

			if len(cfg.EnvironmentFile) > 0 {
				result, err := util.ReadEnvironmentFile(cfg.EnvironmentFile)
				if err != nil {
					glog.Warningf("Unable to read %s: %v", cfg.EnvironmentFile, err)
				} else {
					cfg.Environment = result
				}
			}

			envs, err := parseEnvs(cmd, "env")
			checkErr(err)
			for k, v := range envs {
				cfg.Environment[k] = v
			}

			if len(oldScriptsFlag) != 0 {
				glog.Warning("Flag --scripts is deprecated, use --scripts-url instead")
				cfg.ScriptsURL = oldScriptsFlag
			}
			if len(oldDestination) != 0 {
				glog.Warning("Flag --location is deprecated, use --destination instead")
				cfg.Destination = oldDestination
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

	buildCmd.Flags().BoolVarP(&(cfg.Quiet), "quiet", "q", false, "Operate quietly. Suppress all non-error output.")
	buildCmd.Flags().BoolVar(&(cfg.Incremental), "incremental", false, "Perform an incremental build")
	buildCmd.Flags().BoolVar(&(cfg.RemovePreviousImage), "rm", false, "Remove the previous image during incremental builds")
	buildCmd.Flags().StringP("env", "e", "", "Specify an environment var NAME=VALUE,NAME2=VALUE2,...")
	buildCmd.Flags().StringVarP(&(cfg.Ref), "ref", "r", "", "Specify a ref to check-out")
	buildCmd.Flags().StringVar(&(cfg.CallbackURL), "callback-url", "", "Specify a URL to invoke via HTTP POST upon build completion")
	buildCmd.Flags().StringVarP(&(cfg.ScriptsURL), "scripts-url", "s", "", "Specify a URL for the assemble and run scripts")
	buildCmd.Flags().StringVar(&(oldScriptsFlag), "scripts", "", "Specify a URL for the assemble and run scripts")
	buildCmd.Flags().StringVarP(&(oldDestination), "location", "l", "", "Specify a destination location for untar operation")
	buildCmd.Flags().StringVarP(&(cfg.Destination), "destination", "d", "", "Specify a destination location for untar operation")
	buildCmd.Flags().BoolVar(&(cfg.ForcePull), "force-pull", true, "Always pull the builder image even if it is present locally")
	buildCmd.Flags().BoolVar(&(cfg.PreserveWorkingDir), "save-temp-dir", false, "Save the temporary directory used by STI instead of deleting it")
	buildCmd.Flags().BoolVar(&(useConfig), "use-config", false, "Store command line options to .stifile")
	buildCmd.Flags().StringVarP(&(cfg.ContextDir), "context-dir", "", "", "Specify the sub-directory inside the repository with the application sources")
	buildCmd.Flags().StringVarP(&(cfg.DockerCfgPath), "dockercfg-path", "", filepath.Join(os.Getenv("HOME"), ".dockercfg"), "Specify the path to the Docker configuration file")
	buildCmd.Flags().StringVarP(&(cfg.EnvironmentFile), "environment-file", "E", "", "Specify the path to the file with environment")
	buildCmd.Flags().StringVarP(&(cfg.DisplayName), "application-name", "n", "", "Specify the display name for the application (default: output image name)")
	buildCmd.Flags().StringVarP(&(cfg.Description), "description", "", "", "Specify the description of the application")

	return buildCmd
}

func newCmdCreate() *cobra.Command {
	return &cobra.Command{
		Use:   "create <imageName> <destination>",
		Short: "Bootstrap a new STI image repository",
		Long:  "Bootstrap a new STI image with given imageName inside the destination directory",
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

			cfg.BuilderImage = args[0]
			envs, err := parseEnvs(cmd, "env")
			checkErr(err)
			cfg.Environment = envs

			if len(oldScriptsFlag) != 0 {
				glog.Warning("Flag --scripts is deprecated, use --scripts-url instead")
				cfg.ScriptsURL = oldScriptsFlag
			}

			uh, err := sti.NewUsage(cfg)
			checkErr(err)
			err = uh.Show()
			checkErr(err)
		},
	}
	usageCmd.Flags().StringP("env", "e", "", "Specify an environment var NAME=VALUE,NAME2=VALUE2,...")
	usageCmd.Flags().StringVarP(&(cfg.ScriptsURL), "scripts-url", "s", "", "Specify a URL for the assemble and run scripts")
	usageCmd.Flags().StringVar(&(oldScriptsFlag), "scripts", "", "Specify a URL for the assemble and run scripts")
	usageCmd.Flags().BoolVar(&(cfg.ForcePull), "force-pull", true, "Always pull the builder image even if it is present locally")
	usageCmd.Flags().BoolVar(&(cfg.PreserveWorkingDir), "save-temp-dir", false, "Save the temporary directory used by STI instead of deleting it")
	usageCmd.Flags().StringVarP(&(oldDestination), "location", "l", "", "Specify a destination location for untar operation")
	usageCmd.Flags().StringVarP(&(cfg.Destination), "destination", "d", "", "Specify a destination location for untar operation")
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
	// the preferred for STI would be to redirect to stdout
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
		glog.Error("If the problem persists consult the docs at https://github.com/openshift/source-to-image/tree/master/docs." +
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
		Use: "sti",
		Long: "Source-to-image (STI) is a tool for building repeatable docker images.\n\n" +
			"A command line interface that injects and assembles source code into a docker image.\n" +
			"Complete documentation is available at http://github.com/openshift/source-to-image",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}
	cfg.DockerConfig = defaultDockerConfig()
	stiCmd.PersistentFlags().StringVarP(&(cfg.DockerConfig.Endpoint), "url", "U", cfg.DockerConfig.Endpoint, "Set the url of the docker socket to use")
	stiCmd.PersistentFlags().StringVar(&(cfg.DockerConfig.CertFile), "cert", cfg.DockerConfig.CertFile, "Set the path of the docker TLS certificate file")
	stiCmd.PersistentFlags().StringVar(&(cfg.DockerConfig.KeyFile), "key", cfg.DockerConfig.KeyFile, "Set the path of the docker TLS key file")
	stiCmd.PersistentFlags().StringVar(&(cfg.DockerConfig.CAFile), "ca", cfg.DockerConfig.CAFile, "Set the path of the docker TLS ca file")

	stiCmd.AddCommand(newCmdVersion())
	stiCmd.AddCommand(newCmdBuild(cfg))
	stiCmd.AddCommand(newCmdUsage(cfg))
	stiCmd.AddCommand(newCmdCreate())
	setupGlog(stiCmd.PersistentFlags())

	err := stiCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

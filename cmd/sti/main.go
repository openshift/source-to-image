package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/build/strategies"
	"github.com/openshift/source-to-image/pkg/build/strategies/sti"
	"github.com/openshift/source-to-image/pkg/config"
	"github.com/openshift/source-to-image/pkg/create"
	"github.com/openshift/source-to-image/pkg/docker"
	"github.com/openshift/source-to-image/pkg/errors"
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

func validRequest(r *api.Request) bool {
	return !(len(r.Source) == 0 || len(r.BaseImage) == 0)
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

func newCmdBuild(req *api.Request) *cobra.Command {
	useConfig := false

	buildCmd := &cobra.Command{
		Use:   "build <source> <image> [<tag>]",
		Short: "Build a new image",
		Long:  "Build a new Docker image named <tag> (if provided) from a source repository and base image.",
		Run: func(cmd *cobra.Command, args []string) {
			// Attempt to restore the build command from the configuration file
			if useConfig {
				config.Restore(req, cmd)
			}

			// If user specifies the arguments, then we override the stored ones
			if len(args) >= 2 {
				req.Source = args[0]
				req.BaseImage = args[1]
				if len(args) >= 3 {
					req.Tag = args[2]
				}
			}

			if !validRequest(req) {
				cmd.Help()
				os.Exit(1)
			}

			// Persists the current command line options and request into .stifile
			if useConfig {
				config.Save(req, cmd)
			}

			// Attempt to read the .dockercfg and extract the authentication for
			// docker pull
			if r, err := os.Open(req.DockerCfgPath); err == nil {
				req.PullAuthentication = docker.GetImageRegistryAuth(r, req.BaseImage)
			}

			envs, err := parseEnvs(cmd, "env")
			checkErr(err)
			req.Environment = envs

			if glog.V(2) {
				fmt.Printf("\n%s\n", req.PrintObj())
			}
			builder, err := strategies.GetStrategy(req)
			checkErr(err)
			result, err := builder.Build(req)
			checkErr(err)

			for _, message := range result.Messages {
				glog.V(1).Infof(message)
			}

		},
	}

	buildCmd.Flags().BoolVarP(&(req.Quiet), "quiet", "q", false, "Operate quietly. Suppress all non-error output.")
	buildCmd.Flags().BoolVar(&(req.Incremental), "incremental", false, "Perform an incremental build")
	buildCmd.Flags().BoolVar(&(req.RemovePreviousImage), "rm", false, "Remove the previous image during incremental builds")
	buildCmd.Flags().StringP("env", "e", "", "Specify an environment var NAME=VALUE,NAME2=VALUE2,...")
	buildCmd.Flags().StringVarP(&(req.Ref), "ref", "r", "", "Specify a ref to check-out")
	buildCmd.Flags().StringVar(&(req.CallbackURL), "callbackURL", "", "Specify a URL to invoke via HTTP POST upon build completion")
	buildCmd.Flags().StringVarP(&(req.ScriptsURL), "scripts", "s", "", "Specify a URL for the assemble and run scripts")
	buildCmd.Flags().StringVarP(&(req.Location), "location", "l", "", "Specify a destination location for untar operation")
	buildCmd.Flags().BoolVar(&(req.ForcePull), "forcePull", true, "Always pull the builder image even if it is present locally")
	buildCmd.Flags().BoolVar(&(req.PreserveWorkingDir), "saveTempDir", false, "Save the temporary directory used by STI instead of deleting it")
	buildCmd.Flags().BoolVar(&(useConfig), "use-config", false, "Store command line options to .stifile")
	buildCmd.Flags().StringVarP(&(req.ContextDir), "contextDir", "", "", "Specify the sub-directory inside the repository with the application sources")
	buildCmd.Flags().StringVarP(&(req.DockerCfgPath), "dockerCfgPath", "", filepath.Join(os.Getenv("HOME"), ".dockercfg"), "Specify the path to the Docker configuration file")

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

func newCmdUsage(req *api.Request) *cobra.Command {
	usageCmd := &cobra.Command{
		Use:   "usage <image>",
		Short: "Print usage of the assemble script associated with the image",
		Long:  "Create and start a container from the image and invoke its usage script.",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				cmd.Help()
				os.Exit(1)
			}

			req.BaseImage = args[0]
			envs, err := parseEnvs(cmd, "env")
			checkErr(err)
			req.Environment = envs

			uh, err := sti.NewUsage(req)
			checkErr(err)
			err = uh.Show()
			checkErr(err)
		},
	}
	usageCmd.Flags().StringP("env", "e", "", "Specify an environment var NAME=VALUE,NAME2=VALUE2,...")
	usageCmd.Flags().StringVarP(&(req.ScriptsURL), "scripts", "s", "", "Specify a URL for the assemble and run scripts")
	usageCmd.Flags().BoolVar(&(req.ForcePull), "forcePull", true, "Always pull the builder image even if it is present locally")
	usageCmd.Flags().BoolVar(&(req.PreserveWorkingDir), "saveTempDir", false, "Save the temporary directory used by STI instead of deleting it")
	usageCmd.Flags().StringVarP(&(req.Location), "location", "l", "", "Specify a destination location for untar operation")
	return usageCmd
}

func setupGlog(flags *pflag.FlagSet) {
	from := flag.CommandLine
	if fflag := from.Lookup("v"); fflag != nil {
		level := fflag.Value.(*glog.Level)
		levelPtr := (*int32)(level)
		flags.Int32Var(levelPtr, "loglevel", 0, "Set the level of log output (0-3)")
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
	req := &api.Request{}
	stiCmd := &cobra.Command{
		Use: "sti",
		Long: "Source-to-image (STI) is a tool for building repeatable docker images.\n\n" +
			"A command line interface that injects and assembles source code into a docker image.\n" +
			"Complete documentation is available at http://github.com/openshift/source-to-image",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}
	req.DockerConfig = defaultDockerConfig()
	stiCmd.PersistentFlags().StringVarP(&(req.DockerConfig.Endpoint), "url", "U", req.DockerConfig.Endpoint, "Set the url of the docker socket to use")
	stiCmd.PersistentFlags().StringVar(&(req.DockerConfig.CertFile), "cert", req.DockerConfig.CertFile, "Set the path of the docker TLS certificate file")
	stiCmd.PersistentFlags().StringVar(&(req.DockerConfig.KeyFile), "key", req.DockerConfig.KeyFile, "Set the path of the docker TLS key file")
	stiCmd.PersistentFlags().StringVar(&(req.DockerConfig.CAFile), "ca", req.DockerConfig.CAFile, "Set the path of the docker TLS ca file")

	stiCmd.AddCommand(newCmdVersion())
	stiCmd.AddCommand(newCmdBuild(req))
	stiCmd.AddCommand(newCmdUsage(req))
	stiCmd.AddCommand(newCmdCreate())
	setupGlog(stiCmd.PersistentFlags())

	err := stiCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/openshift/source-to-image/pkg/sti"
	"github.com/openshift/source-to-image/pkg/sti/api"
	"github.com/openshift/source-to-image/pkg/sti/errors"
	"github.com/openshift/source-to-image/pkg/sti/version"
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

func dockerSocket() string {
	if host := os.Getenv("DOCKER_HOST"); host != "" {
		return host
	}
	return "unix:///var/run/docker.sock"
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
	buildCmd := &cobra.Command{
		Use:   "build <source> <image> <tag>",
		Short: "Build a new image",
		Long:  "Build a new Docker image named <tag> from a source repository and base image.",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) < 3 {
				cmd.Help()
				os.Exit(1)
			}

			req.Source = args[0]
			req.BaseImage = args[1]
			req.Tag = args[2]
			envs, err := parseEnvs(cmd, "env")
			checkErr(err)
			req.Environment = envs

			b, err := sti.NewBuilder(req)
			checkErr(err)
			res, err := b.Build()
			checkErr(err)
			for _, message := range res.Messages {
				glog.V(1).Infof(message)
			}
		},
	}
	buildCmd.Flags().BoolVar(&(req.Clean), "clean", false, "Perform a clean build")
	buildCmd.Flags().BoolVar(&(req.RemovePreviousImage), "rm", false, "Remove the previous image during incremental builds")
	buildCmd.Flags().StringP("env", "e", "", "Specify an environment var NAME=VALUE,NAME2=VALUE2,...")
	buildCmd.Flags().StringVarP(&(req.Ref), "ref", "r", "", "Specify a ref to check-out")
	buildCmd.Flags().StringVar(&(req.CallbackURL), "callbackURL", "", "Specify a URL to invoke via HTTP POST upon build completion")
	buildCmd.Flags().StringVarP(&(req.ScriptsURL), "scripts", "s", "", "Specify a URL for the assemble and run scripts")
	buildCmd.Flags().StringVarP(&(req.Location), "location", "l", "", "Specify a destination location for untar operation")
	buildCmd.Flags().BoolVar(&(req.ForcePull), "forcePull", true, "Always pull the builder image even if it is present locally")
	buildCmd.Flags().BoolVar(&(req.PreserveWorkingDir), "saveTempDir", false, "Save the temporary directory used by STI instead of deleting it")
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
			b := sti.NewCreate(args[0], args[1])
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
		glog.Fatalf("An error occurred: %v", err)
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
	stiCmd.PersistentFlags().StringVarP(&(req.DockerSocket), "url", "U", dockerSocket(), "Set the url of the docker socket to use")

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

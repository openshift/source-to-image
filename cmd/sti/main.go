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
	} else {
		return "unix:///var/run/docker.sock"
	}
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

func newCmdBuild(req *sti.STIRequest) *cobra.Command {
	buildCmd := &cobra.Command{
		Use:   "build <source> <image> <tag>",
		Short: "Build a new image",
		Long:  "Build a new Docker image named <tag> from a source repository and base image.",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) < 3 {
				glog.Fatalf("Valid arguments: <source> <image> <tag> ...")
			}

			req.Source = args[0]
			req.BaseImage = args[1]
			req.Tag = args[2]
			envs, err := parseEnvs(cmd, "env")
			if err != nil {
				glog.Fatalf("An error occured: %v", err)
			}
			req.Environment = envs

			b, err := sti.NewBuilder(req)
			if err != nil {
				glog.Fatalf("An error occured: %v", err)
			}
			res, err := b.Build()
			if err != nil {
				glog.Fatalf("An error occured: %v", err)
			}
			for _, message := range res.Messages {
				glog.V(1).Infof(message)
			}
		},
	}
	buildCmd.Flags().BoolVar(&(req.Clean), "clean", false, "Perform a clean build")
	buildCmd.Flags().BoolVar(&(req.RemovePreviousImage), "rm", false, "Remove the previous image during incremental builds")
	buildCmd.Flags().StringP("env", "e", "", "Specify an environment var NAME=VALUE,NAME2=VALUE2,...")
	buildCmd.Flags().StringVarP(&(req.Ref), "ref", "r", "", "Specify a ref to check-out")
	buildCmd.Flags().StringVar(&(req.CallbackUrl), "callbackUrl", "", "Specify a URL to invoke via HTTP POST upon build completion")
	buildCmd.Flags().StringVarP(&(req.ScriptsUrl), "scripts", "s", "", "Specify a URL for the assemble and run scripts")
	buildCmd.Flags().BoolVar(&(req.ForcePull), "forcePull", true, "Always pull the builder image even if it is present locally")
	return buildCmd
}

func newCmdUsage(req *sti.STIRequest) *cobra.Command {
	usageCmd := &cobra.Command{
		Use:   "usage <image>",
		Short: "Print usage of the assemble script associated with the image",
		Long:  "Create and start a container from the image and invoke it's usage (run `assemble -h' script).",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				glog.Fatalf("Valid arguments: <image>")
			}

			req.BaseImage = args[0]
			envs, err := parseEnvs(cmd, "env")
			if err != nil {
				glog.Fatalf(err.Error())
			}
			req.Environment = envs

			uh, err := sti.NewUsage(req)
			if err != nil {
				glog.Fatalf("An error occurred: %v", err)
			}
			err = uh.Show()
			if err != nil {
				glog.Fatalf("An error occurred: %v", err)
			}
		},
	}
	usageCmd.Flags().StringP("env", "e", "", "Specify an environment var NAME=VALUE,NAME2=VALUE2,...")
	usageCmd.Flags().StringVarP(&(req.ScriptsUrl), "scripts", "s", "", "Specify a URL for the assemble and run scripts")
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
	// the preferred for STI wouuld be to redirect to stdout
	flag.CommandLine.Set("logtostderr", "true")
}

func main() {
	req := &sti.STIRequest{}
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
	stiCmd.PersistentFlags().BoolVar(&(req.PreserveWorkingDir), "savetempdir", false, "Save the temporary directory used by STI instead of deleting it")

	stiCmd.AddCommand(newCmdVersion())
	stiCmd.AddCommand(newCmdBuild(req))
	stiCmd.AddCommand(newCmdUsage(req))
	setupGlog(stiCmd.PersistentFlags())

	if err := stiCmd.Execute(); err != nil {
		glog.Fatalf("Error: %s", err)
	}
}

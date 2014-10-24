package main

import (
	"fmt"
	"log"
	_ "net/http/pprof"
	"os"
	"strings"

	"github.com/openshift/source-to-image/pkg/sti"
	"github.com/spf13/cobra"
)

var version string

func parseEnvs(envStr string) (map[string]string, error) {
	if envStr == "" {
		return nil, nil
	}

	envs := make(map[string]string)
	pairs := strings.Split(envStr, ",")

	for _, pair := range pairs {
		atoms := strings.Split(pair, "=")

		if len(atoms) != 2 {
			return nil, fmt.Errorf("Malformed env string: %s", pair)
		}

		name := atoms[0]
		value := atoms[1]

		envs[name] = value
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

// Execute runs the main STI command
func Execute() {
	var (
		req       *sti.STIRequest
		envString string
	)

	req = &sti.STIRequest{}

	stiCmd := &cobra.Command{
		Use: "sti",
		Long: "Source-to-image (STI) is a tool for building repeatable docker images.\n\n" +
			"A command line interface that injects and assembles source code into a docker image.\n" +
			"Complete documentation is available at http://github.com/openshift/source-to-image",
		Run: func(c *cobra.Command, args []string) {
			c.Help()
		},
	}
	stiCmd.PersistentFlags().StringVarP(&(req.DockerSocket), "url", "U", dockerSocket(), "Set the url of the docker socket to use")
	stiCmd.PersistentFlags().BoolVar(&(req.Verbose), "verbose", false, "Enable verbose output")
	stiCmd.PersistentFlags().BoolVar(&(req.PreserveWorkingDir), "savetempdir", false, "Save the temporary directory used by STI instead of deleting it")

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Display version",
		Long:  "Display version",
		Run: func(c *cobra.Command, args []string) {
			fmt.Printf("sti %s\n", version)
		},
	}

	stiCmd.AddCommand(versionCmd)

	buildCmd := &cobra.Command{
		Use:   "build <source> <image> <tag>",
		Short: "Build a new image",
		Long:  "Build a new Docker image named <tag> from a source repository and base image.",
		Run: func(c *cobra.Command, args []string) {
			// if we're not verbose, make sure the logger doesn't print out timestamps
			if !req.Verbose {
				log.SetFlags(0)
			}

			if len(args) < 3 {
				fmt.Println("Valid arguments: <source> <image> <tag> ...")
				os.Exit(1)
			}

			req.Source = args[0]
			req.BaseImage = args[1]
			req.Tag = args[2]

			envs, err := parseEnvs(envString)
			if err != nil {
				fmt.Printf(err.Error())
				os.Exit(1)
			}
			req.Environment = envs

			b, err := sti.NewBuilder(req)
			if err != nil {
				fmt.Printf("An error occured: %s\n", err.Error())
				os.Exit(1)
			}
			res, err := b.Build()
			if err != nil {
				fmt.Printf("An error occured: %s\n", err.Error())
				os.Exit(1)
			}

			for _, message := range res.Messages {
				fmt.Println(message)
			}

		},
	}
	buildCmd.Flags().BoolVar(&(req.Clean), "clean", false, "Perform a clean build")
	buildCmd.Flags().BoolVar(&(req.RemovePreviousImage), "rm", false, "Remove the previous image during incremental builds")
	buildCmd.Flags().StringVarP(&envString, "env", "e", "", "Specify an environment var NAME=VALUE,NAME2=VALUE2,...")
	buildCmd.Flags().StringVarP(&(req.Ref), "ref", "r", "", "Specify a ref to check-out")
	buildCmd.Flags().StringVar(&(req.CallbackUrl), "callbackUrl", "", "Specify a URL to invoke via HTTP POST upon build completion")
	buildCmd.Flags().StringVarP(&(req.ScriptsUrl), "scripts", "s", "", "Specify a URL for the assemble and run scripts")

	stiCmd.AddCommand(buildCmd)

	usageCmd := &cobra.Command{
		Use:   "usage <image>",
		Short: "Print usage of the assemble script associated with the image",
		Long:  "Create and start a container from the image and invoke it's usage (run `assemble -h' script).",
		Run: func(c *cobra.Command, args []string) {
			// if we're not verbose, make sure the logger doesn't print out timestamps
			if !req.Verbose {
				log.SetFlags(0)
			}

			if len(args) == 0 {
				fmt.Println("Valid arguments: <image>")
				os.Exit(1)
			}

			req.BaseImage = args[0]

			envs, err := parseEnvs(envString)
			if err != nil {
				fmt.Println(err.Error())
				os.Exit(1)
			}

			req.Environment = envs

			uh, err := sti.NewUsage(req)
			if err != nil {
				fmt.Printf("An error occurred: %s\n", err.Error())
				os.Exit(1)
			}
			err = uh.Show()
			if err != nil {
				fmt.Printf("An error occured: %s\n", err.Error())
				os.Exit(1)
			}
		},
	}
	usageCmd.Flags().StringVarP(&envString, "env", "e", "", "Specify an environment var NAME=VALUE,NAME2=VALUE2,...")
	usageCmd.Flags().StringVarP(&(req.ScriptsUrl), "scripts", "s", "", "Specify a URL for the assemble and run scripts")

	stiCmd.AddCommand(usageCmd)

	stiCmd.Execute()
}

func main() {
	Execute()
}

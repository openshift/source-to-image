package main

import (
	"log"
	_ "net/http/pprof"

	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	sti "github.com/openshift/docker-source-to-images/go"
	"github.com/spf13/cobra"
)

func parseEnvs(envStr string) (map[string]string, error) {
	if envStr == "" {
		return nil, nil
	}

	envs := make(map[string]string)
	pairs := strings.Split(envStr, ",")

	for _, pair := range pairs {
		atoms := strings.Split(pair, "=")

		if len(atoms) != 2 {
			return nil, errors.New("Malformed env string: " + pair)
		}

		name := atoms[0]
		value := atoms[1]

		envs[name] = value
	}

	return envs, nil
}

func Execute() {
	var (
		req       sti.Request
		envString string
		buildReq  sti.BuildRequest
	)

	stiCmd := &cobra.Command{
		Use:   "sti",
		Short: "STI is a tool for building repeatable docker images",
		Long: `A command-line interface for the sti library
              Complete documentation is available at http://github.com/pmorie/go-sti`,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}
	stiCmd.PersistentFlags().StringVarP(&(req.DockerSocket), "url", "U", "unix:///var/run/docker.sock", "Set the url of the docker socket to use")
	stiCmd.PersistentFlags().BoolVar(&(req.Verbose), "verbose", false, "Enable verbose output")

	buildCmd := &cobra.Command{
		Use:   "build SOURCE BUILD_IMAGE APP_IMAGE_TAG",
		Short: "Build an image",
		Long:  "Build an image",
		Run: func(cmd *cobra.Command, args []string) {
			// if we're not verbose, make sure the logger doesn't print out timestamps
			if !req.Verbose {
				log.SetFlags(0)
			}

			if len(args) == 0 {
				cmd.Usage()
				return
			}

			buildReq.Request = req
			buildReq.Source = args[0]
			buildReq.BaseImage = args[1]
			buildReq.Tag = args[2]
			buildReq.Writer = os.Stdout

			envs, _ := parseEnvs(envString)
			buildReq.Environment = envs

			if buildReq.WorkingDir == "tempdir" {
				var err error
				buildReq.WorkingDir, err = ioutil.TempDir("", "sti")
				if err != nil {
					fmt.Println(err.Error())
					return
				}
				defer os.Remove(buildReq.WorkingDir)
			}

			res, err := sti.Build(buildReq)
			if err != nil {
				fmt.Printf("An error occured: %s\n", err.Error())
				return
			}

			for _, message := range res.Messages {
				fmt.Println(message)
			}
		},
	}
	buildCmd.Flags().BoolVar(&(buildReq.Clean), "clean", false, "Perform a clean build")
	buildCmd.Flags().StringVar(&(req.WorkingDir), "dir", "tempdir", "Directory where generated Dockerfiles and other support scripts are created")
	buildCmd.Flags().StringVarP(&envString, "env", "e", "", "Specify an environment var NAME=VALUE,NAME2=VALUE2,...")
	buildCmd.Flags().StringVarP(&(buildReq.Ref), "ref", "r", "", "Specify a ref to check-out")
	buildCmd.Flags().StringVar(&(buildReq.CallbackUrl), "callbackUrl", "", "Specify a URL to invoke via HTTP POST upon build completion")
	buildCmd.Flags().StringVarP(&(buildReq.ScriptsUrl), "scripts", "s", "", "Specify a URL for the assemble and run scripts")

	stiCmd.AddCommand(buildCmd)
	stiCmd.Execute()
}

func main() {
	Execute()
}

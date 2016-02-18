package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openshift/source-to-image/pkg/api"
	"github.com/spf13/cobra"
)

// AddCommonFlags adds the common flags for usage, build and rebuild commands
func AddCommonFlags(c *cobra.Command, cfg *api.Config) {
	c.Flags().BoolVarP(&(cfg.Quiet), "quiet", "q", false,
		"Operate quietly. Suppress all non-error output.")
	c.Flags().BoolVar(&(cfg.Incremental), "incremental", false,
		"Perform an incremental build")
	c.Flags().BoolVar(&(cfg.RemovePreviousImage), "rm", false,
		"Remove the previous image during incremental builds")
	c.Flags().StringVar(&(cfg.CallbackURL), "callback-url", "",
		"Specify a URL to invoke via HTTP POST upon build completion")
	c.Flags().BoolVar(&(cfg.ForcePull), "force-pull", false,
		"DEPRECATED: Always pull the builder image even if it is present locally")
	c.Flags().VarP(&(cfg.BuilderPullPolicy), "pull-policy", "p",
		"Specify when to pull the builder image (always, never or if-not-present)")
	c.Flags().Var(&(cfg.PreviousImagePullPolicy), "incremental-pull-policy",
		"Specify when to pull the previous image for incremental builds (always, never or if-not-present)")
	c.Flags().BoolVar(&(cfg.PreserveWorkingDir), "save-temp-dir", false,
		"Save the temporary directory used by S2I instead of deleting it")
	c.Flags().StringVarP(&(cfg.DockerCfgPath), "dockercfg-path", "", filepath.Join(os.Getenv("HOME"), ".docker/config.json"),
		"Specify the path to the Docker configuration file")
	c.Flags().StringVarP(&(cfg.Destination), "destination", "d", "",
		"Specify a destination location for untar operation")
}

// ParseEnvs parses the command line environemnt variable definitions
func ParseEnvs(c *cobra.Command, name string) (map[string]string, error) {
	env := c.Flags().Lookup(name)
	if env == nil || len(env.Value.String()) == 0 {
		return nil, nil
	}
	envs := make(map[string]string)
	pairs := strings.Split(env.Value.String(), ",")
	for _, pair := range pairs {
		atoms := strings.Split(pair, "=")
		if len(atoms) != 2 {
			return nil, fmt.Errorf("malformed syntax for environment variable: %s", pair)
		}
		envs[atoms[0]] = atoms[1]
	}
	return envs, nil
}

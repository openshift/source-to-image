package util

import (
	"fmt"

	"github.com/docker/engine-api/types/container"
)

// SafeForLoggingContainerConfig returns a string version of the container.Config object
// with sensitive information (proxy environment variables containing credentials)
// redacted.
func SafeForLoggingContainerConfig(config *container.Config) string {
	strippedEnv := StripProxyCredentials(config.Env)
	newConfig := *config
	newConfig.Env = strippedEnv
	return fmt.Sprintf("%+v", newConfig)
}

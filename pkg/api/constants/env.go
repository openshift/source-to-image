package constants

// Deprecated Docker ENV variables
const (
	// LocationEnvironment is the environment variable that specifies where to place artifacts in a builder image.
	//
	// DEPRECATED - use DestinationLabel instead.
	LocationEnvironment = "STI_LOCATION"
	// ScriptsURLEnvironment is the environment variable name that specifies where to look for S2I scripts.
	//
	// DEPRECATED - use ScriptsURLLabel instead.
	ScriptsURLEnvironment = "STI_SCRIPTS_URL"

	// ContainerManagerEnv is the environment variable name that specifies the container runtime S2I will use to
	// build the application's image.
	ContainerManagerEnv = "S2I_CONTAINER_MANAGER"
)

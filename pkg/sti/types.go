package sti

// Request contains essential fields for any request: a base image, source directory,
// and tag
type Request struct {
	BaseImage           string
	DockerSocket        string
	PreserveWorkingDir  bool
	Source              string
	Ref                 string
	Tag                 string
	Clean               bool
	RemovePreviousImage bool
	Environment         map[string]string
	CallbackURL         string
	ScriptsURL          string
	ForcePull           bool

	incremental bool
	workingDir  string
}

// Result includes a flag that indicates whether the build was successful
// and if an image was created, the image ID
type Result struct {
	Success    bool
	Messages   []string
	WorkingDir string
	ImageID    string
}

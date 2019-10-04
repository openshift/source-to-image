package external

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
	"syscall"
	"text/template"

	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/build/strategies/dockerfile"
	"github.com/openshift/source-to-image/pkg/util/fs"
	utillog "github.com/openshift/source-to-image/pkg/util/log"
)

// External represents the shell out for external build commands, therefore s2i based build can
// execute the generation of
type External struct {
	dockerfile *dockerfile.Dockerfile
}

// s2iDockerfile Dockerfile default filename.
const s2iDockerfile = "Dockerfile.s2i"

var (
	// local logger
	log = utillog.StderrLog
	// supported external commands, template is based on api.Config instance
	commands = map[string]string{
		"buildah": `buildah bud --tag {{ .Tag }} --file {{ .AsDockerfile }} {{ or .WorkingDir "." }}`,
		"docker":  `docker build --tag {{ .Tag }} --file {{ .AsDockerfile }} {{ or .WorkingDir "." }}`,
		"podman":  `podman build --tag {{ .Tag }} --file {{ .AsDockerfile }} {{ or .WorkingDir "." }}`,
	}
)

// GetBuilders returns a list of command names, based global commands map.
func GetBuilders() []string {
	builders := []string{}
	for k := range commands {
		builders = append(builders, k)
	}
	sort.Strings(builders)
	return builders
}

// ValidBuilderName returns a boolean based in keys of global commands map.
func ValidBuilderName(name string) bool {
	_, exists := commands[name]
	return exists
}

// renderCommand render a shell command based in api.Config instance. Attribute WithBuilder
// wll determine external builder name, and api.Config feeds command's template variables. It can
// return error in case of template parsing or evaluation issues.
func (e *External) renderCommand(config *api.Config) (string, error) {
	commandTemplate, exists := commands[config.WithBuilder]
	if !exists {
		return "", fmt.Errorf("cannot find command '%s' in dictionary: '%#v'",
			config.WithBuilder, commands)
	}

	t, err := template.New("external-command").Parse(commandTemplate)
	if err != nil {
		return "", err
	}
	var output bytes.Buffer
	if err = t.Execute(&output, config); err != nil {
		return "", err
	}
	return output.String(), nil
}

// execute the given external command using "os/exec". Returns the outcomes as api.Result, making
// sure it only marks result as success when exit-code is zero. Therefore, it returns errors based
// in external command errors, so "s2i build" also fails.
func (e *External) execute(externalCommand string) (*api.Result, error) {
	log.V(0).Infof("Executing external build command: '%s'", externalCommand)

	externalCommandSlice := strings.Split(externalCommand, " ")
	cmd := exec.Command(externalCommandSlice[0], externalCommandSlice[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	res := &api.Result{Success: false}
	res.Messages = append(res.Messages, fmt.Sprintf("Running command: '%s'", externalCommand))
	err := cmd.Start()
	if err != nil {
		res.Messages = append(res.Messages, err.Error())
		return res, err
	}

	if err := cmd.Wait(); err != nil {
		if exitErr, okay := err.(*exec.ExitError); okay {
			if status, okay := exitErr.Sys().(syscall.WaitStatus); okay {
				exitCode := status.ExitStatus()
				log.V(0).Infof("External command return-code: %d", exitCode)
				res.Messages = append(res.Messages, fmt.Sprintf("exit-code: %d", exitCode))
				if exitCode == 0 {
					res.Success = true
				} else {
					return res, exitErr
				}
			}
		} else {
			return res, err
		}
	}
	return res, nil
}

// asDockerfile inspect config, if user has already informed `--as-dockerfile` option, that's simply
// returned, otherwise, considering `--working-dir` option first before using artificial name.
func (e *External) asDockerfile(config *api.Config) string {
	if len(config.AsDockerfile) > 0 {
		return config.AsDockerfile
	}

	if len(config.WorkingDir) > 0 {
		return path.Join(config.WorkingDir, s2iDockerfile)
	}
	return s2iDockerfile
}

// Build triggers the build of a "strategy/dockerfile" to obtain "AsDockerfile" first, and then
// proceed to execute the external command.
func (e *External) Build(config *api.Config) (*api.Result, error) {
	config.AsDockerfile = e.asDockerfile(config)

	externalCommand, err := e.renderCommand(config)
	if err != nil {
		return nil, err
	}

	// generating dockerfile following AsDockerfile directive
	err = e.dockerfile.CreateDockerfile(config)
	if err != nil {
		return nil, err
	}

	return e.execute(externalCommand)
}

// New instance of External command strategy.
func New(config *api.Config, fs fs.FileSystem) (*External, error) {
	df, err := dockerfile.New(config, fs)
	if err != nil {
		return nil, err
	}
	return &External{dockerfile: df}, nil
}

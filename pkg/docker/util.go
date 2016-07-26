package docker

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/errors"
	utilglog "github.com/openshift/source-to-image/pkg/util/glog"
	"github.com/openshift/source-to-image/pkg/util/user"
)

// glog is a placeholder until the builders pass an output stream down
// client facing libraries should not be using glog
var glog = utilglog.StderrLog

// ImageReference points to a Docker image.
type ImageReference struct {
	Registry  string
	Namespace string
	Name      string
	Tag       string
	ID        string
}

type AuthConfigurations struct {
	Configs map[string]api.AuthConfig
}

type dockerConfig struct {
	Auth  string `json:"auth"`
	Email string `json:"email"`
}

const (
	// maxErrorOutput is the maximum length of the error output saved for processing
	maxErrorOutput  = 1024
	defaultRegistry = "https://index.docker.io/v1/"
)

// GetImageRegistryAuth retrieves the appropriate docker client authentication object for a given
// image name and a given set of client authentication objects.
func GetImageRegistryAuth(auths *AuthConfigurations, imageName string) api.AuthConfig {
	glog.V(5).Infof("Getting docker credentials for %s", imageName)
	spec, err := ParseImageReference(imageName)
	if err != nil {
		glog.V(0).Infof("error: Failed to parse docker reference %s", imageName)
		return api.AuthConfig{}
	}

	if auth, ok := auths.Configs[spec.Registry]; ok {
		glog.V(5).Infof("Using %s[%s] credentials for pulling %s", auth.Email, spec.Registry, imageName)
		return auth
	}
	if auth, ok := auths.Configs[defaultRegistry]; ok {
		glog.V(5).Infof("Using %s credentials for pulling %s", auth.Email, imageName)
		return auth
	}
	return api.AuthConfig{}
}

// LoadImageRegistryAuth loads and returns the set of client auth objects from a docker config
// json file.
func LoadImageRegistryAuth(dockerCfg io.Reader) *AuthConfigurations {
	auths, err := NewAuthConfigurations(dockerCfg)
	if err != nil {
		glog.V(0).Infof("error: Unable to load docker config")
		return nil
	}
	return auths
}

// begin borrowed from fsouza
func NewAuthConfigurations(r io.Reader) (*AuthConfigurations, error) {
	var auth *AuthConfigurations
	confs, err := parseDockerConfig(r)
	if err != nil {
		return nil, err
	}
	auth, err = authConfigs(confs)
	if err != nil {
		return nil, err
	}
	return auth, nil
}

func parseDockerConfig(r io.Reader) (map[string]dockerConfig, error) {
	buf := new(bytes.Buffer)
	buf.ReadFrom(r)
	byteData := buf.Bytes()

	confsWrapper := struct {
		Auths map[string]dockerConfig `json:"auths"`
	}{}
	if err := json.Unmarshal(byteData, &confsWrapper); err == nil {
		if len(confsWrapper.Auths) > 0 {
			return confsWrapper.Auths, nil
		}
	}

	var confs map[string]dockerConfig
	if err := json.Unmarshal(byteData, &confs); err != nil {
		return nil, err
	}
	return confs, nil
}

// authConfigs converts a dockerConfigs map to a AuthConfigurations object.
func authConfigs(confs map[string]dockerConfig) (*AuthConfigurations, error) {
	c := &AuthConfigurations{
		Configs: make(map[string]api.AuthConfig),
	}
	for reg, conf := range confs {
		data, err := base64.StdEncoding.DecodeString(conf.Auth)
		if err != nil {
			return nil, err
		}
		userpass := strings.SplitN(string(data), ":", 2)
		if len(userpass) != 2 {
			return nil, fmt.Errorf("cannot parse username/password from %s", userpass)
		}
		c.Configs[reg] = api.AuthConfig{
			Email:         conf.Email,
			Username:      userpass[0],
			Password:      userpass[1],
			ServerAddress: reg,
		}
	}
	return c, nil
}

// end borrowed from fsouza

// LoadAndGetImageRegistryAuth loads the set of client auth objects from a docker config file
// and returns the appropriate client auth object for a given image name.
func LoadAndGetImageRegistryAuth(dockerCfg io.Reader, imageName string) api.AuthConfig {
	auths, err := NewAuthConfigurations(dockerCfg)
	if err != nil {
		glog.V(0).Infof("error: Unable to load docker config")
		return api.AuthConfig{}
	}
	return GetImageRegistryAuth(auths, imageName)
}

// StreamContainerIO takes data from the Reader and redirects to the log function (typically we pass in
// glog.Error for stderr and glog.Info for stdout
func StreamContainerIO(errStream io.Reader, errOutput *string, log func(...interface{})) {
	scanner := bufio.NewReader(errStream)
	for {
		text, err := scanner.ReadString('\n')
		if err != nil {
			// we're ignoring ErrClosedPipe, as this is information
			// the docker container ended streaming logs
			if glog.Is(2) && err != io.ErrClosedPipe && err != io.EOF {
				glog.V(0).Infof("error: Error reading docker stderr, %v", err)
			}
			break
		}
		log(text)
		if errOutput != nil && len(*errOutput) < maxErrorOutput {
			*errOutput += text + "\n"
		}
	}
}

// ParseImageReference parses a Docker pull spec string into a ImageReference.
// FIXME: This code was copied from OpenShift repository.
func ParseImageReference(spec string) (ImageReference, error) {
	var ref ImageReference

	// TODO replace with docker version once docker/docker PR11109 is merged upstream
	stream, tag, id := parseRepositoryTag(spec)

	repoParts := strings.Split(stream, "/")
	switch len(repoParts) {
	case 2:
		if strings.Contains(repoParts[0], ":") {
			// registry/name
			ref.Registry = repoParts[0]
			ref.Namespace = "library"
			ref.Name = repoParts[1]
			ref.Tag = tag
			ref.ID = id
			return ref, nil
		}
		// namespace/name
		ref.Namespace = repoParts[0]
		ref.Name = repoParts[1]
		ref.Tag = tag
		ref.ID = id
		return ref, nil
	case 3:
		// registry/namespace/name
		ref.Registry = repoParts[0]
		ref.Namespace = repoParts[1]
		ref.Name = repoParts[2]
		ref.Tag = tag
		ref.ID = id
		return ref, nil
	case 1:
		// name
		if len(repoParts[0]) == 0 {
			return ref, fmt.Errorf("the docker pull spec %q must be two or three segments separated by slashes", spec)
		}
		ref.Name = repoParts[0]
		ref.Tag = tag
		ref.ID = id
		return ref, nil
	default:
		return ref, fmt.Errorf("the docker pull spec %q must be two or three segments separated by slashes", spec)
	}
}

// TODO remove (base, tag, id)
func parseRepositoryTag(repos string) (string, string, string) {
	n := strings.Index(repos, "@")
	if n >= 0 {
		parts := strings.Split(repos, "@")
		return parts[0], "", parts[1]
	}
	n = strings.LastIndex(repos, ":")
	if n < 0 {
		return repos, "", ""
	}
	if tag := repos[n+1:]; !strings.Contains(tag, "/") {
		return repos[:n], tag, ""
	}
	return repos, "", ""
}

// PullImage pulls the Docker image specifies by name taking the pull policy
// into the account.
// TODO: The 'force' option will be removed
func PullImage(name string, d Docker, policy api.PullPolicy, force bool) (*PullResult, error) {
	// TODO: Remove this after we deprecate --force-pull
	if force {
		policy = api.PullAlways
	}

	if len(policy) == 0 {
		return nil, fmt.Errorf("the policy for pull image must be set")
	}

	var (
		image *api.Image
		err   error
	)
	switch policy {
	case api.PullIfNotPresent:
		image, err = d.CheckAndPullImage(name)
	case api.PullAlways:
		glog.Infof("Pulling image %q ...", name)
		image, err = d.PullImage(name)
	case api.PullNever:
		glog.Infof("Checking if image %q is available locally ...", name)
		image, err = d.CheckImage(name)
	}
	return &PullResult{Image: image, OnBuild: d.IsImageOnBuild(name)}, err
}

// CheckAllowedUser retrieves the user for a Docker image and checks that user against
// an allowed range of uids.
// - If the range of users is not empty, then the user on the Docker image needs to be a numeric user
// - The user's uid must be contained by the range(s) specified by the uids Rangelist
// - If the image contains ONBUILD instructions and those instructions also contain a USER directive,
//   then the user specified by that USER directive must meet the uid range criteria as well.
func CheckAllowedUser(d Docker, imageName string, uids user.RangeList, isOnbuild bool) error {
	if uids == nil || uids.Empty() {
		return nil
	}
	imageUserSpec, err := d.GetImageUser(imageName)
	if err != nil {
		return err
	}
	imageUser := extractUser(imageUserSpec)
	if !user.IsUserAllowed(imageUser, &uids) {
		return errors.NewUserNotAllowedError(imageName, false)
	}
	if isOnbuild {
		cmds, err := d.GetOnBuild(imageName)
		if err != nil {
			return err
		}
		if !isOnbuildAllowed(cmds, &uids) {
			return errors.NewUserNotAllowedError(imageName, true)
		}
	}
	return nil
}

var dockerLineDelim = regexp.MustCompile(`[\t\v\f\r ]+`)

// isOnbuildAllowed checks a list of Docker ONBUILD instructions for
// user directives. It ensures that any users specified by the directives
// falls within the specified range list of users.
func isOnbuildAllowed(directives []string, allowed *user.RangeList) bool {
	for _, line := range directives {
		parts := dockerLineDelim.Split(line, 2)
		if strings.ToLower(parts[0]) != "user" {
			continue
		}
		uname := extractUser(parts[1])
		if !user.IsUserAllowed(uname, allowed) {
			return false
		}
	}
	return true
}

func extractUser(userSpec string) string {
	user := userSpec
	if strings.Contains(user, ":") {
		parts := strings.SplitN(userSpec, ":", 2)
		user = parts[0]
	}
	return strings.TrimSpace(user)
}

// IsReachable returns true if the Docker daemon is reachable from s2i
func IsReachable(config *api.Config) bool {
	d, err := New(config.DockerConfig, config.PullAuthentication)
	if err != nil {
		return false
	}
	return d.Ping() == nil
}

// GetBuilderImage processes the config and performs operations necessary to make
// the Docker image specified as BuilderImage available locally.
// It returns information about the base image, containing metadata necessary
// for choosing the right STI build strategy.
func GetBuilderImage(config *api.Config) (*PullResult, error) {
	d, err := New(config.DockerConfig, config.PullAuthentication)
	if err != nil {
		return nil, err
	}

	r, err := PullImage(config.BuilderImage, d, config.BuilderPullPolicy, config.ForcePull)
	if err != nil {
		return nil, err
	}

	err = CheckAllowedUser(d, config.BuilderImage, config.AllowedUIDs, r.OnBuild)
	if err != nil {
		return nil, err
	}

	return r, nil
}

// GetRuntimeImage processes the config and performs operations necessary to make
// the Docker image specified as RuntimeImage available locally.
func GetRuntimeImage(config *api.Config, docker Docker) error {
	pullPolicy := config.RuntimeImagePullPolicy
	if len(pullPolicy) == 0 {
		pullPolicy = api.DefaultRuntimeImagePullPolicy
	}

	pullResult, err := PullImage(config.RuntimeImage, docker, pullPolicy, false)
	if err != nil {
		return err
	}

	err = CheckAllowedUser(docker, config.RuntimeImage, config.AllowedUIDs, pullResult.OnBuild)
	if err != nil {
		return err
	}

	return nil
}

func GetDefaultDockerConfig() *api.DockerConfig {
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

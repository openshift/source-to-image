package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/docker/client"

	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/api/constants"
	"github.com/openshift/source-to-image/pkg/util/user"
)

func rangeList(str string) *user.RangeList {
	l, err := user.ParseRangeList(str)
	if err != nil {
		panic(err)
	}
	return l
}

func TestCheckAllowedUser(t *testing.T) {
	tests := []struct {
		name         string
		allowedUIDs  *user.RangeList
		user         string
		onbuild      []string
		expectErr    bool
		assembleUser string
		labels       map[string]string
	}{
		{
			name:        "AllowedUIDs is not set",
			allowedUIDs: rangeList(""),
			user:        "root",
			onbuild:     []string{},
			expectErr:   false,
		},
		{
			name:        "AllowedUIDs is set, non-numeric user",
			allowedUIDs: rangeList("0"),
			user:        "default",
			onbuild:     []string{},
			expectErr:   true,
		},
		{
			name:        "AllowedUIDs is set, user 0",
			allowedUIDs: rangeList("1-"),
			user:        "0",
			onbuild:     []string{},
			expectErr:   true,
		},
		{
			name:        "AllowedUIDs is set, numeric user, non-numeric onbuild",
			allowedUIDs: rangeList("1-10,30-"),
			user:        "100",
			onbuild:     []string{"COPY test test", "USER default"},
			expectErr:   true,
		},
		{
			name:        "AllowedUIDs is set, numeric user, no onbuild user directive",
			allowedUIDs: rangeList("1-10,30-"),
			user:        "200",
			onbuild:     []string{"VOLUME /data"},
			expectErr:   false,
		},
		{
			name:        "AllowedUIDs is set, numeric user, onbuild numeric user directive",
			allowedUIDs: rangeList("200,500-"),
			user:        "200",
			onbuild:     []string{"USER 500", "VOLUME /data"},
			expectErr:   false,
		},
		{
			name:        "AllowedUIDs is set, numeric user, onbuild user 0",
			allowedUIDs: rangeList("1-"),
			user:        "200",
			onbuild:     []string{"RUN echo \"hello world\"", "USER 0"},
			expectErr:   true,
		},
		{
			name:        "AllowedUIDs is set, numeric user, onbuild numeric user directive, upper bound range",
			allowedUIDs: rangeList("-1000"),
			user:        "80",
			onbuild:     []string{"USER 501", "VOLUME /data"},
			expectErr:   false,
		},
		{
			name:        "AllowedUIDs is set, numeric user with group",
			allowedUIDs: rangeList("1-"),
			user:        "5:5000",
			expectErr:   false,
		},
		{
			name:        "AllowedUIDs is set, numeric user with named group",
			allowedUIDs: rangeList("1-"),
			user:        "5:group",
			expectErr:   false,
		},
		{
			name:        "AllowedUIDs is set, named user with group",
			allowedUIDs: rangeList("1-"),
			user:        "root:wheel",
			expectErr:   true,
		},
		{
			name:        "AllowedUIDs is set, numeric user, onbuild user with group",
			allowedUIDs: rangeList("1-"),
			user:        "200",
			onbuild:     []string{"RUN echo \"hello world\"", "USER 10:100"},
			expectErr:   false,
		},
		{
			name:        "AllowedUIDs is set, numeric user, onbuild named user with group",
			allowedUIDs: rangeList("1-"),
			user:        "200",
			onbuild:     []string{"RUN echo \"hello world\"", "USER root:wheel"},
			expectErr:   true,
		},
		{
			name:        "AllowedUIDs is set, numeric user, onbuild user with named group",
			allowedUIDs: rangeList("1-"),
			user:        "200",
			onbuild:     []string{"RUN echo \"hello world\"", "USER 10:wheel"},
			expectErr:   false,
		},
		{
			name:         "AllowedUIDs is set, numeric user, assemble user override ok",
			allowedUIDs:  rangeList("1-"),
			user:         "200",
			assembleUser: "10",
			expectErr:    false,
		},
		{
			name:         "AllowedUIDs is set, numeric user, root assemble user",
			allowedUIDs:  rangeList("1-"),
			user:         "200",
			assembleUser: "0",
			expectErr:    true,
		},
		{
			name:        "AllowedUIDs is set, numeric user, assemble user label ok",
			allowedUIDs: rangeList("1-"),
			user:        "200",
			labels:      map[string]string{constants.AssembleUserLabel: "10"},
			expectErr:   false,
		},
		{
			name:        "AllowedUIDs is set, numeric user, assemble user label root",
			allowedUIDs: rangeList("1-"),
			user:        "200",
			labels:      map[string]string{constants.AssembleUserLabel: "0"},
			expectErr:   true,
		},
		{
			name:        "AllowedUIDs is set, root image user, assemble user label ok",
			allowedUIDs: rangeList("1-"),
			user:        "0",
			labels:      map[string]string{constants.AssembleUserLabel: "10"},
			expectErr:   false,
		},
		{
			name:         "AllowedUIDs is set, root image user, assemble user override ok",
			allowedUIDs:  rangeList("1-"),
			user:         "0",
			assembleUser: "10",
			expectErr:    false,
		},
		{
			name:        "AllowedUIDs is set, root image user, onbuild root named user with group, assemble user label ok",
			allowedUIDs: rangeList("1-"),
			user:        "0",
			labels:      map[string]string{constants.AssembleUserLabel: "10"},
			onbuild:     []string{"RUN echo \"hello world\"", "USER root:wheel", "RUN echo \"i am gROOT\"", "USER 10"},
			expectErr:   true,
		},
		{
			name:         "AllowedUIDs is set, root image user, onbuild root named user with group, assemble user override ok",
			allowedUIDs:  rangeList("1-"),
			user:         "0",
			assembleUser: "10",
			onbuild:      []string{"RUN echo \"hello world\"", "USER root:wheel", "RUN echo \"i am gROOT\"", "USER 10"},
			expectErr:    true,
		},
	}

	for _, tc := range tests {
		docker := &FakeDocker{
			GetImageUserResult: tc.user,
			OnBuildResult:      tc.onbuild,
			Labels:             tc.labels,
		}
		err := CheckAllowedUser(docker, "", *tc.allowedUIDs, len(tc.onbuild) > 0, tc.assembleUser)
		if err != nil && !tc.expectErr {
			t.Errorf("%s: unexpected error: %v", tc.name, err)
		}
		if err == nil && tc.expectErr {
			t.Errorf("%s: expected error, but did not get any", tc.name)
		}
	}
}

func TestGetDefaultDockerConfig(t *testing.T) {
	tests := []struct {
		envHost           string
		envCertPath       string
		envTLSVerify      string
		envTLS            string
		expectedHost      string
		expectedCertFile  string
		expectedTLSVerify bool
		expectedTLS       bool
	}{
		{
			envHost:      "tcp://docker:2376",
			envCertPath:  "/expected/cert/path",
			envTLSVerify: "true",
			envTLS:       "true",

			expectedHost:      "tcp://docker:2376",
			expectedCertFile:  "/expected/cert/path/cert.pem",
			expectedTLSVerify: true,
			expectedTLS:       true,
		},
		{
			envHost:      "",
			envCertPath:  "",
			envTLSVerify: "",
			envTLS:       "",

			expectedHost:      "unix:///var/run/docker.sock",
			expectedCertFile:  Dir() + "/cert.pem",
			expectedTLSVerify: false,
			expectedTLS:       false,
		},
	}
	for _, tc := range tests {
		oldXdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
		oldHost := os.Getenv("DOCKER_HOST")
		oldCertPath := os.Getenv("DOCKER_CERT_PATH")
		oldTLSVerify := os.Getenv("DOCKER_TLS_VERIFY")
		oldTLS := os.Getenv("DOCKER_TLS")
		os.Setenv("XDG_RUNTIME_DIR", "")
		os.Setenv("DOCKER_HOST", tc.envHost)
		os.Setenv("DOCKER_CERT_PATH", tc.envCertPath)
		os.Setenv("DOCKER_TLS_VERIFY", tc.envTLSVerify)
		os.Setenv("DOCKER_TLS", tc.envTLS)
		defer os.Setenv("XDG_RUNTIME_DIR", oldXdgRuntimeDir)
		defer os.Setenv("DOCKER_HOST", oldHost)
		defer os.Setenv("DOCKER_CERT_PATH", oldCertPath)
		defer os.Setenv("DOCKER_TLS_VERIFY", oldTLSVerify)
		defer os.Setenv("DOCKER_TLS", oldTLS)

		cfg := GetDefaultDockerConfig()
		if tc.expectedHost != cfg.Endpoint {
			t.Errorf("Endpoint: expected '%s', but got '%s'", tc.expectedHost, cfg.Endpoint)
		}
		if tc.expectedCertFile != cfg.CertFile {
			t.Errorf("CertFile: expected '%s', but got '%s'", tc.expectedCertFile, cfg.CertFile)
		}
		if tc.expectedTLSVerify != cfg.TLSVerify {
			t.Errorf("TLSVerify: expected '%t', but got '%t'", tc.expectedTLSVerify, cfg.TLSVerify)
		}
		if tc.expectedTLS != cfg.UseTLS {
			t.Errorf("UseTLS: expected '%t', but got '%t'", tc.expectedTLS, cfg.UseTLS)
		}
	}
}

func TestGetDefaultContainerEngineHost(t *testing.T) {

	tmpDir, err := os.MkdirTemp("", "s2i-container-engine-host-*")
	if err != nil {
		t.Fatalf("failed to create xdg temp dir: %v", err)
	}
	defer func() {
		if rmErr := os.RemoveAll(tmpDir); rmErr != nil {
			t.Errorf("failed to clean up xdg temp dir: %v", err)
		}
	}()

	testCases := []struct {
		name               string
		xdgRuntimeDir      string
		createPodmanSocket bool
		expectedHost       string
	}{
		{
			name:               "rootless podman - socket exists",
			xdgRuntimeDir:      tmpDir,
			createPodmanSocket: true,
			expectedHost:       fmt.Sprintf("unix://%s", filepath.Join(tmpDir, "podman", "podman.sock")),
		},
		{
			name:               "rootless podman - socket does not exist",
			xdgRuntimeDir:      tmpDir,
			createPodmanSocket: false,
			expectedHost:       client.DefaultDockerHost,
		},
		{
			name:         "docker default",
			expectedHost: client.DefaultDockerHost,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			oldXdgDir := os.Getenv("XDG_RUNTIME_DIR")
			os.Setenv("XDG_RUNTIME_DIR", tc.xdgRuntimeDir)
			defer os.Setenv("XDG_RUNTIME_DIR", oldXdgDir)

			socketCreated := false
			socketPath := filepath.Join(tc.xdgRuntimeDir, "podman", "podman.sock")
			if tc.createPodmanSocket {

				if _, err := os.Stat(socketPath); err != nil {
					if err := os.MkdirAll(filepath.Dir(socketPath), 0750); err != nil {
						t.Fatalf("failed to create socket directory: %v", err)
					}
					file, err := os.Create(socketPath)
					if err != nil {
						t.Fatalf("failed to create default podman socket: %v", err)
					}
					defer func() {
						if closeErr := file.Close(); closeErr != nil {
							t.Errorf("failed to close file: %v", closeErr)
						}
					}()
					socketCreated = true
				}
			}

			engineHost := GetDefaultContainerEngineHost()
			if tc.expectedHost != engineHost {
				t.Errorf("expected container host %s; got %s", tc.expectedHost, engineHost)
			}

			if socketCreated {
				if err := os.RemoveAll(filepath.Dir(socketPath)); err != nil {
					t.Errorf("failed to clean up podman socket: %v", err)
				}
			}

		})
	}
}

func TestGetAssembleUser(t *testing.T) {
	testCases := []struct {
		name              string
		containerUser     string
		assembleUserLabel string
		expectedResult    string
		extractedResult   string
	}{
		{
			name: "empty",
		},
		{
			name:            "container user set",
			containerUser:   "1002",
			expectedResult:  "1002",
			extractedResult: "1002",
		},
		{
			name:            "container user:group set",
			containerUser:   "1002:1002",
			expectedResult:  "1002:1002",
			extractedResult: "1002",
		},
		{
			name:              "assemble user label set",
			assembleUserLabel: "1003",
			expectedResult:    "1003",
			extractedResult:   "1003",
		},
		{
			name:              "assemble user:group label set",
			assembleUserLabel: "1003:1003",
			expectedResult:    "1003:1003",
			extractedResult:   "1003",
		},
		{
			name:              "assemble user override",
			containerUser:     "1002",
			assembleUserLabel: "1003",
			expectedResult:    "1003",
			extractedResult:   "1003",
		},
		{
			name:              "assemble user:group override",
			containerUser:     "1002:1002",
			assembleUserLabel: "1003:1003",
			expectedResult:    "1003:1003",
			extractedResult:   "1003",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeDocker := &FakeDocker{
				GetImageUserResult:        tc.containerUser,
				AssembleRuntimeUserResult: tc.assembleUserLabel,
				Labels:                    make(map[string]string),
			}
			if tc.assembleUserLabel != "" {
				fakeDocker.Labels[constants.AssembleUserLabel] = tc.assembleUserLabel
			}
			assembleUser, err := GetAssembleUser(fakeDocker, &api.Config{BuilderImage: "dummy-image:latest"})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if assembleUser != tc.expectedResult {
				t.Fatalf("expected assemble user %s, got %s", tc.expectedResult, assembleUser)
			}
			extractedUser := extractUser(assembleUser)
			if extractedUser != tc.extractedResult {
				t.Fatalf("expected extracted user %s, got %s", tc.extractedResult, extractedUser)
			}
		})
	}
}

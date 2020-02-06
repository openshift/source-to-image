package external

import (
	"reflect"
	"testing"

	"github.com/openshift/source-to-image/pkg/api"
)

func TestExternalGetBuilders(t *testing.T) {
	tests := []struct {
		name string
		want []string
	}{
		{"builders", []string{"buildah", "docker", "podman"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetBuilders(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetBuilders() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExternalValidBuilderName(t *testing.T) {
	tests := []struct {
		name string
		args string
		want bool
	}{
		{"valid-builder-name", "buildah", true},
		{"valid-builder-name", "docker", true},
		{"valid-builder-name", "podman", true},
		{"invalid-builder-name", "other", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidBuilderName(tt.args); got != tt.want {
				t.Errorf("ValidBuilderName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExternalRenderCommand(t *testing.T) {
	e := &External{}
	tests := []struct {
		name    string
		config  *api.Config
		want    string
		wantErr bool
	}{
		{
			"render-buildah",
			&api.Config{WithBuilder: "buildah", Tag: "tag", AsDockerfile: "dockerfile"},
			"buildah bud --tag tag --file dockerfile .",
			false,
		},
		{
			"render-podman",
			&api.Config{WithBuilder: "podman", Tag: "tag", AsDockerfile: "dockerfile"},
			"podman build --tag tag --file dockerfile .",
			false,
		},
		{
			"render-docker",
			&api.Config{WithBuilder: "docker", Tag: "tag", AsDockerfile: "dockerfile"},
			"docker build --tag tag --file dockerfile .",
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := e.renderCommand(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("External.renderCommand() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("External.renderCommand() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExternalAsDockerfile(t *testing.T) {
	e := &External{}
	tests := []struct {
		name   string
		config *api.Config
		want   string
	}{
		{
			"without-as-dockerfile-without-working-dir",
			&api.Config{},
			"Dockerfile.s2i",
		},
		{
			"without-as-dockerfile-with-working-dir",
			&api.Config{WorkingDir: "dir"},
			"dir/Dockerfile.s2i",
		},
		{
			"with-as-dockerfile-with-working-dir",
			&api.Config{AsDockerfile: "Dockerfile", WorkingDir: "dir"},
			"Dockerfile",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := e.asDockerfile(tt.config); got != tt.want {
				t.Errorf("External.asDockerfile() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExternal_execute(t *testing.T) {
	e := &External{}
	tests := []struct {
		name            string
		externalCommand string
		want            bool
		wantErr         bool
	}{
		{
			"successful-true-command",
			"/usr/bin/true",
			true,
			false,
		},
		{
			"error-false-command",
			"/usr/bin/false",
			false,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := e.execute(tt.externalCommand)
			if (err != nil) != tt.wantErr {
				t.Errorf("External.execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want != got.Success {
				t.Errorf("External.execute() = %v, want %v", got, tt.want)
			}
		})
	}
}

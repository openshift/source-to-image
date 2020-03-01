// Package buildah implements docker.Docker interface in order to support buildah as an alternative
// container runtime system for s2i. It consumes buildah through "os/exec" calls, composing
// command-line with arguments in order to execute s2i workflows.
package buildah

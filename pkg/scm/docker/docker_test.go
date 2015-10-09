package docker

import "testing"

func TestParseDockerSource(t *testing.T) {
	table := map[string][]string{
		"docker://foo/bar":                  {"foo/bar", "."},
		"docker://foo/bar:":                 {"foo/bar", "."},
		"openshift/ruby-20-centos:/tmp/dir": {"openshift/ruby-20-centos", "/tmp/dir"},
		"docker://openshift/app-image:/":    {"openshift/app-image", "/"},
		"docker://openshift/app-image:.":    {"openshift/app-image", "."},
		"docker://openshift/app-image:":     {"openshift/app-image", "."},
		"docker:///bar":                     {"", ""},
	}
	for source, result := range table {
		image, location := parseDockerSource(source)
		if image != result[0] {
			t.Errorf("Expected %q to return image name %q, got %q", source, result[0], image)
		}
		if location != result[1] {
			t.Errorf("Expected %q to return location %q, got %q", source, result[1], location)
		}
	}
}

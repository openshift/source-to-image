package buildah

import (
	"testing"
)

func TestExecute(t *testing.T) {
	output, err := Execute([]string{"ls", "-1"}, nil, true)
	if err != nil {
		t.Fatalf("execute returned error '%#v'", err)
	}
	if len(output) == 0 {
		t.Fatal("empty output, where command output is expected instead")
	}
}

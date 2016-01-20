package util

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/openshift/source-to-image/pkg/api"
)

func TestExpandInjectedFiles(t *testing.T) {
	tmp, err := ioutil.TempDir("", "s2i-test-")
	tmpNested, err := ioutil.TempDir(tmp, "nested")
	if err != nil {
		t.Errorf("Unable to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmp)
	list := api.InjectionList{{SourcePath: tmp, DestinationDir: "/foo"}}
	f1, _ := ioutil.TempFile(tmp, "foo")
	f2, _ := ioutil.TempFile(tmpNested, "bar")
	files, err := ExpandInjectedFiles(list)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	expected := []string{"/foo/" + filepath.Base(f1.Name()), filepath.Join("/foo", filepath.Base(tmpNested), filepath.Base(f2.Name()))}
	for _, exp := range expected {
		found := false
		for _, f := range files {
			if f == exp {
				found = true
			}
		}
		if !found {
			t.Errorf("Expected %q in resulting file list, got %+v", exp, files)
		}
	}
}

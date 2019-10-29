package util

import (
	"github.com/openshift/source-to-image/pkg/api"
	"github.com/openshift/source-to-image/pkg/api/constants"
	"io/ioutil"
	"os"
	"path/filepath"

	"testing"
)

func TestImageMetadataLabels(t *testing.T) {
	tests := []struct {
		json  string
		count int
	}{
		{
			json:  "{\"labels\": [{\"org.tenkichannel/service\":\"rain-forecast-process\"}]}",
			count: 1,
		},
		{
			json:  "{\"labels\": [{\"labelkey1\":\"value1\"},{\"labelkey2\":\"value2\"}]}",
			count: 2,
		},
		{
			json:  "{\"labels\": [{\"labelkey1\":\"value1\",\"labelkey2\":\"value2\"}]}",
			count: 2,
		},
	}
	for _, tc := range tests {
		tempDir, err := ioutil.TempDir("", "image_metadata")
		if err != nil {
			t.Fatalf("could not create temp dir: %v", err)
		}
		defer os.RemoveAll(tempDir)
		err = os.Chmod(tempDir, 0777)
		if err != nil {
			t.Fatalf("could not chmod temp dir: %v", err)
		}
		err = os.MkdirAll(filepath.Join(tempDir, constants.SourceConfig), 0777)
		if err != nil {
			t.Fatalf("could not create subdirs: %v", err)
		}

		path := filepath.Join(tempDir, constants.SourceConfig, MetadataFilename)
		err = ioutil.WriteFile(path, []byte(tc.json), 0700)
		if err != nil {
			t.Fatalf("could not create temp image_metadata.json: %v", err)
		}

		cfg := &api.Config{
			WorkingDir: tempDir,
		}
		data := GenerateOutputImageLabels(nil, cfg)
		if len(data) != tc.count {
			t.Fatalf("data from GenerateOutputImageLabels len %d when needed %d for %s", len(data), tc.count, tc.json)
		}

	}

}

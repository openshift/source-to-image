package scripts

import (
	"testing"

	"github.com/openshift/source-to-image/pkg/api"
)

func TestConvertEnvironment(t *testing.T) {
	env := []Environment{
		{"FOO", "BAR"},
	}
	result := ConvertEnvironment(env)
	if len(result) != 1 {
		t.Errorf("Expected 1 item, got %d", len(result))
	}
	if result[0] != "FOO=BAR" {
		t.Errorf("Expected FOO=BAR, got %v", result)
	}
}

func TestConvertEnvironmentList(t *testing.T) {
	testEnv := api.EnvironmentList{
		{Name: "Key1", Value: "Value1"},
		{Name: "Key2", Value: "Value2"},
		{Name: "Key3", Value: "Value3"},
		{Name: "Key4", Value: "Value=4"},
		{Name: "Key5", Value: "Value,5"},
	}
	result := ConvertEnvironmentList(testEnv)
	expected := []string{"Key1=Value1", "Key2=Value2", "Key3=Value3", "Key4=Value=4", "Key5=Value,5"}
	if !equalArrayContents(result, expected) {
		t.Errorf("Unexpected result. Expected: %#v. Actual: %#v",
			expected, result)
	}
}

func equalArrayContents(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, e := range a {
		found := false
		for _, f := range b {
			if f == e {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

package util

import (
	"testing"
)

func TestStripProxyCredentials(t *testing.T) {

	inputs := []string{
		"http_proxy=user:password@hostname.com",
		"https_proxy=user:password@hostname.com",
		"HTTP_PROXY=user:password@hostname.com",
		"HTTPS_PROXY=user:password@hostname.com",
		"http_proxy=http://user:password@hostname.com",
		"https_proxy=https://user:password@hostname.com",
		"HTTP_PROXY=http://user:password@hostname.com",
		"HTTPS_PROXY=https://user:password@hostname.com",
		"http_proxy=http://hostname.com",
		"https_proxy=https://hostname.com",
		"HTTP_PROXY=http://hostname.com",
		"HTTPS_PROXY=https://hostname.com",
		"othervalue=user:password@hostname.com",
	}

	expected := []string{
		"http_proxy=hostname.com",
		"https_proxy=hostname.com",
		"HTTP_PROXY=hostname.com",
		"HTTPS_PROXY=hostname.com",
		"http_proxy=hostname.com",
		"https_proxy=hostname.com",
		"HTTP_PROXY=hostname.com",
		"HTTPS_PROXY=hostname.com",
		"http_proxy=http://hostname.com",
		"https_proxy=https://hostname.com",
		"HTTP_PROXY=http://hostname.com",
		"HTTPS_PROXY=https://hostname.com",
		"othervalue=user:password@hostname.com",
	}
	result := StripProxyCredentials(inputs)
	for i := range result {
		if result[i] != expected[i] {
			t.Errorf("expected %s to be stripped to %s, but got %s", inputs[i], expected[i], result[i])
		}
	}
}

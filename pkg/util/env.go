package util

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

// ReadEnvironmentFile reads the content for a file that contains a list of
// environment variables and values. The key-pairs are separated by a new line
// character. The file can also have comments (both '#' and '//' are supported).
func ReadEnvironmentFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	result := map[string]string{}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		s := strings.TrimSpace(scanner.Text())
		// Allow for comments in environment file
		if strings.HasPrefix(s, "#") || strings.HasPrefix(s, "//") {
			continue
		}
		parts := strings.SplitN(s, "=", 2)
		if len(parts) != 2 {
			continue
		}
		result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}

	return result, scanner.Err()
}

// StripProxyCredentials attempts to strip sensitive information from proxy
// environment variables.
func StripProxyCredentials(env []string) []string {
	// case insensitively match all key=value variables containing the word "proxy"
	// in the key and which appear to contain a user:password@host pattern.  We'll
	// keep everything before the = sign, and after the @.
	re := regexp.MustCompile(`(?i)(.*proxy.*=).*@(.*)`)
	newEnv := make([]string, len(env))
	copy(newEnv, env)
	for i, val := range newEnv {
		newEnv[i] = re.ReplaceAllString(val, "$1$2")
	}
	return newEnv
}

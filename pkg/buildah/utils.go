package buildah

import (
	"strings"
)

// chompBytesToString remove new line from informed bytes payload, returning as string.
func chompBytesToString(in []byte) string {
	return strings.TrimRight(string(in), "\n")
}

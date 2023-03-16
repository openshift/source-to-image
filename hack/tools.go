//go:build tools
// +build tools

package hack

// Add tools that scripts depend on here, to ensure they are vendored.
import (
	_ "github.com/go-imports-organizer/goio"
)

// Package buildinfo contains values shared by the standalone CLI surfaces.
package buildinfo

import "strings"

var (
	// Version is the application release version embedded in the binary.
	// Keep this as the single source of truth for user-visible version reporting.
	Version = "0.4.21-dev"

	// Credit identifies the project maintainer shown by the CLI metadata command.
	Credit = "aklkbqx"
)

// IsDevVersion reports whether the given version string carries a -dev suffix.
func IsDevVersion(v string) bool {
	trimmed := strings.TrimSpace(v)
	return strings.HasSuffix(strings.ToLower(trimmed), "-dev")
}

// IsDevelopment reports whether the embedded buildinfo.Version is a development build.
func IsDevelopment() bool {
	return IsDevVersion(Version)
}

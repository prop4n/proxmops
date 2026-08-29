// Package version exposes build metadata, populated at link time via -ldflags.
package version

import "fmt"

// Build information, overridden at build time, for example:
//
//	go build -ldflags "-X github.com/prop4n/proxmops/internal/version.Version=v0.1.0"
var (
	// Version is the semantic version of the build.
	Version = "dev"
	// Commit is the git commit the binary was built from.
	Commit = "none"
	// Date is the build timestamp in RFC 3339 form.
	Date = "unknown"
)

// String returns a human-readable one-line description of the build.
func String() string {
	return fmt.Sprintf("proxmops %s (commit %s, built %s)", Version, Commit, Date)
}

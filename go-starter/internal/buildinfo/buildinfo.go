// Package buildinfo carries version metadata that is injected at link time
// with -ldflags "-X github.com/codingseeker/go-starter/internal/buildinfo.Version=...".
package buildinfo

// These values are overridden by the linker; keep the defaults meaningful for
// local builds and tests.
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

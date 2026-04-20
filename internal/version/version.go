package version

// Injected at build time via -ldflags; fall back to sentinel values for go run / untagged builds.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

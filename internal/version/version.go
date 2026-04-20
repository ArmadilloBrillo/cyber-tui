package version

// Version, Commit, and Date are injected at build time via -ldflags.
// When built without ldflags (e.g. go run .), they fall back to these defaults.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

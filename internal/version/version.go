package version

// Version is set at build time via -ldflags:
//
//	go build -ldflags="-X github.com/OpenNSW/nsw-srilanka/internal/version.Version=1.2.3"
//
// Defaults to "dev" for local builds.
var Version = "dev"

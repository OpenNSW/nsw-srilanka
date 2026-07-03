package version

// version is set at build time via -ldflags:
//
//	go build -ldflags="-X github.com/OpenNSW/nsw-srilanka/internal/version.version=1.2.3"
//
// Defaults to "dev" for local builds.
var version = "dev"

// Get returns the embedded application build version.
func Get() string {
	return version
}

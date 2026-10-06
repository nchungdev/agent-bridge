// Package version holds what this binary is. The release workflow sets both values at build time:
//
//	go build -ldflags "-X github.com/nchungdev/agent-bridge/internal/version.Version=v1.0.1 \
//	                   -X github.com/nchungdev/agent-bridge/internal/version.Commit=abc1234"
//
// A plain `go build` is a development build and reports "dev".
package version

var (
	// Version is the release tag this binary was built from (v1.0.1, v1.0.1-b02), or "dev".
	Version = "dev"
	// Commit is the short commit hash it was built from, when known.
	Commit = ""
)

// Installed reports whether this is a release build, which updates itself from GitHub Releases. A development
// build (run from a checkout) updates by git instead.
func Installed() bool { return Version != "" && Version != "dev" }

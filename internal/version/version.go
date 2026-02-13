// Package version provides version information for the Claude Code Proxy.
// The Version variable is injected at build time via -ldflags.
package version

// Version is the current version of the Claude Code Proxy.
// This is injected at build time via:
//   go build -ldflags="-X github.com/claude-code-proxy/proxy/internal/version.Version=x.y.z"
// If not set, defaults to "dev".
var Version = "dev"

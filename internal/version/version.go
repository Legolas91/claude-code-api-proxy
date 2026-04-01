// Package version provides version information for the Claude Code Proxy.
// The Version and Commit variables are injected at build time via -ldflags.
package version

// Version is the current version of the Claude Code Proxy.
// This is injected at build time via:
//
//	go build -ldflags="-X github.com/claude-code-proxy/proxy/internal/version.Version=x.y.z"
//
// If not set, defaults to "dev".
var Version = "dev"

// Commit is the git commit hash at build time.
// This is injected at build time via:
//
//	go build -ldflags="-X github.com/claude-code-proxy/proxy/internal/version.Commit=abc1234"
//
// If not set, defaults to "unknown".
var Commit = "unknown"

// Package version exposes the build version, stamped via -ldflags.
package version

// Version is overridden at build time:
//
//	go build -ldflags "-X github.com/ocrosby/jsonplaceholder-mcp-server/internal/version.Version=1.2.3"
var Version = "dev"

// Package version holds the build metadata surfaced by `tessera version` and
// `--version`. The values originate in package main (set via -ldflags by
// goreleaser) and are passed down through the CLI.
package version

// Info carries build metadata for display.
type Info struct {
	Version string
	Commit  string
	Date    string
}

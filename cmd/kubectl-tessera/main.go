// Command kubectl-tessera mints ephemeral, scope-narrowed, TTL-bound
// Kubernetes credentials. It is a kubectl plugin, invoked as `kubectl tessera`.
package main

import "github.com/wagneripjr/kubectl-tessera/internal/cli"

// Build metadata, injected at release time via -ldflags by goreleaser.
// Defaults apply to `go install` / local builds before the first tag.
var (
	version = "v0.0.0-dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cli.Execute(cli.BuildInfo{Version: version, Commit: commit, Date: date})
}

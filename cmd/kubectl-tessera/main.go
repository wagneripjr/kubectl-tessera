package main

import "github.com/wagneripjr/kubectl-tessera/internal/cli"

var (
	version = "v0.0.0-dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cli.Execute(cli.BuildInfo{Version: version, Commit: commit, Date: date})
}

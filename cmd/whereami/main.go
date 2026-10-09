// Command whereami prints a concise summary of the project you are standing in.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/zahidhasann88/whereami/internal/cli"
)

// Set at build time with -ldflags (see Makefile and .goreleaser.yaml).
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := cli.Execute(ctx, os.Args[1:], os.Stdout, os.Stderr, cli.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	})
	stop()
	os.Exit(code)
}

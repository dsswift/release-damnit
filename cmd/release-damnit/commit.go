package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/dsswift/release-damnit/internal/commit"
)

func runCommit(args []string) int {
	fs := flag.NewFlagSet("commit", flag.ContinueOnError)
	staged := fs.Bool("staged", false, "Plan only what is already staged")
	yes := fs.Bool("yes", false, "Create the commits without asking")
	dryRun := fs.Bool("dry-run", false, "Show the plan and stop")
	model := fs.String("model", "haiku", "Claude model that writes the plan")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), `Usage: release-damnit commit [options]

Commit the pending changes (modified, deleted, renamed and untracked files)
as conventional commits. Claude groups the files and writes the messages; the
plan must pass the commit rules, with one package per commit. Files that may
hold secrets (appsettings.json, local.settings.json, docker-compose.yml, .env,
.env.*, credentials.json) are skipped and never read. Hooks run. Nothing is
pushed; land does that.

Options:`)
		fs.PrintDefaults()
	}
	if code, ok := parseFlags(fs, args); !ok {
		return code
	}
	if fs.NArg() > 0 {
		fs.Usage()
		return 2
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, commit.Prefix+err.Error())
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	err = commit.Run(ctx, commit.Options{
		Dir:     wd,
		Staged:  *staged,
		Yes:     *yes,
		DryRun:  *dryRun,
		Planner: commit.Claude{Model: *model},
		In:      os.Stdin,
		Out:     os.Stdout,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, commit.Prefix+err.Error())
		return 1
	}
	return 0
}

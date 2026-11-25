package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/dsswift/release-damnit/internal/commit"
)

func runCommit(args []string) int {
	fs := flag.NewFlagSet("commit", flag.ContinueOnError)
	staged := fs.Bool("staged", false, "Plan only what is already staged")
	yes := fs.Bool("yes", false, "Create the commits without asking")
	dryRun := fs.Bool("dry-run", false, "Show the plan and stop")
	model := fs.String("model", "haiku", "Claude model that writes the plan")
	var work workItems
	fs.Var(&work, "work", "Item every commit is part of, e.g. APPW-15, added as a \"Work:\" trailer (repeatable, or comma separated); required where vantage.yaml names a work kind")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), `Usage: release-damnit commit [options]

Commit the pending changes (modified, deleted, renamed and untracked files)
as conventional commits. Claude groups the files and writes the messages; the
plan must pass the commit rules, with one package per commit. Files that may
hold secrets (appsettings.json, local.settings.json, docker-compose.yml, .env,
.env.*, credentials.json) are skipped and never read. Hooks run. Nothing is
pushed; land does that. In a repo whose vantage.yaml names a work kind,
--work names the item, and every commit ends with its "Work:" trailer.

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
		Work:    work,
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

// workItems collects --work values: the flag repeats, and each may list
// several items comma separated.
type workItems []string

func (w *workItems) String() string { return strings.Join(*w, ", ") }

func (w *workItems) Set(value string) error {
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			*w = append(*w, item)
		}
	}
	return nil
}

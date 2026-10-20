// release-damnit is a drop-in replacement for Release Please that correctly
// traverses merge commits, plus the commit tooling around it: a commit-msg
// check, a landing command, hook installation, publishing, and scaffolding.
//
// The first argument picks a command. Anything that is not a command name
// runs the original flag-based release command, unchanged.
package main

import (
	"fmt"
	"os"
)

var (
	version = "dev"
	gitSha  = "unknown"
)

// commands maps a subcommand name to its handler. Each handler gets the
// arguments after the name and returns the process exit code.
var commands = map[string]func(args []string) int{
	"check-commit":  runCheckCommit,
	"check-commits": runCheckCommits,
}

func main() {
	if len(os.Args) > 1 {
		if run, ok := commands[os.Args[1]]; ok {
			os.Exit(run(os.Args[2:]))
		}
	}
	runDefault()
}

func printHelp() {
	fmt.Println(`release-damnit - Drop-in replacement for Release Please with correct merge traversal

Usage:
  release-damnit [options]           Version and changelog every package a push changed
  release-damnit <command> [options]

Commands:
  check-commit <msgfile>   Check a commit message against the staged files (commit-msg hook)
  check-commits <range>    Check every non-merge commit in a revision range

  Run release-damnit <command> -h for a command's options.

Options:
  --dry-run          Show what would be done without making changes
  --create-releases  Create provider releases
  --provider NAME     auto, github, or gitlab (default: auto)
  --gitlab-token TOKEN GitLab project access token (or GITLAB_RELEASE_TOKEN)
  --repo-url URL     Repository URL (auto-detected if not provided)
  --since SHA        Analyze every commit after SHA, the branch tip before the
                     push. Ignored when SHA is not an ancestor of HEAD.
  --report-file PATH Write component release report JSON
  --verbose          Show detailed analysis output (unmatched directories, commit details)
  --version          Show version information
  --help             Show this help

Environment Variables:
  GITHUB_OUTPUT      Path to GitHub Actions output file
  GITLAB_RELEASE_TOKEN GitLab project access token for GitLab releases
  CI_API_V4_URL, CI_PROJECT_ID  GitLab CI API context

Examples:
  # See what would be released
  release-damnit --dry-run

  # Update versions and changelogs
  release-damnit

  # Also create GitHub releases
  release-damnit --create-releases

  # Debug: show why commits weren't matched to packages
  release-damnit --dry-run --verbose`)
}

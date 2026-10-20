package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/dsswift/release-damnit/internal/config"
	"github.com/dsswift/release-damnit/internal/scaffold"
)

const initPrefix = "release-damnit init: "

func runInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	forge := fs.String("forge", "github", "Where the repo is hosted: github or gitlab")
	component := fs.String("component", "", "Package name (default: the repo directory name)")
	ver := fs.String("version", "", "Current version (default: highest v* tag, else package.json version, else 0.0.0)")
	force := fs.Bool("force", false, "Overwrite existing files and a foreign commit-msg hook")
	var extra pathList
	fs.Var(&extra, "extra-file", "File that carries the version: json:<path>:<jsonpath> or generic:<path> (repeatable)")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), `Usage: release-damnit init [options]

Set a single repo up to release with release-damnit: writes
release-please-config.json, release-please-manifest.json, VERSION (when no
extra file carries the version), CHANGELOG.md (when missing) and the release
pipeline (GitHub: .github/workflows/release.yml; GitLab: .gitlab-ci.yml and,
when missing, a starting .gitlab/build.yml), then installs the commit-msg hook.

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
	switch scaffold.Forge(*forge) {
	case scaffold.ForgeGitHub, scaffold.ForgeGitLab:
	default:
		fmt.Fprintf(os.Stderr, "%s--forge %q is not supported; use github or gitlab\n", initPrefix, *forge)
		return 1
	}

	extraFiles := make([]config.ExtraFile, 0, len(extra))
	for _, e := range extra {
		f, err := scaffold.ParseExtraFile(e)
		if err != nil {
			fmt.Fprintln(os.Stderr, initPrefix+err.Error())
			return 1
		}
		extraFiles = append(extraFiles, f)
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, initPrefix+err.Error())
		return 1
	}
	result, err := scaffold.Run(scaffold.Options{Dir: wd, Component: *component, Version: *ver, ExtraFiles: extraFiles, Force: *force, Forge: scaffold.Forge(*forge)})
	if err != nil {
		fmt.Fprintln(os.Stderr, initPrefix+err.Error())
		return 1
	}
	fmt.Printf("%s%s at %s\n", initPrefix, result.Component, result.Version)
	for _, path := range result.Written {
		fmt.Printf("%swrote %s\n", initPrefix, path)
	}
	if err := installHooks(wd, *force); err != nil {
		fmt.Fprintln(os.Stderr, hooksPrefix+err.Error())
		return 1
	}
	if scaffold.Forge(*forge) == scaffold.ForgeGitLab {
		fmt.Printf("%sthe tag pipeline runs .gitlab/build.yml: put the repo's own build there ($CI_COMMIT_TAG is the release tag)\n", initPrefix)
		fmt.Printf("%sadd a masked CI/CD variable GITLAB_RELEASE_TOKEN: a project access token with the api and write_repository scopes and a role allowed to push to the default branch\n", initPrefix)
		return 0
	}
	fmt.Printf("%sthe release workflow calls .github/workflows/build.yml: add it as the repo's own build, a workflow_call workflow that takes a string input \"tag\" and checks out that tag\n", initPrefix)
	return 0
}

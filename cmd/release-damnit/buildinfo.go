package main

import "runtime/debug"

// buildVersion reports the version and commit this binary was built from.
// ldflags set them for release builds; `go install module@version` sets no
// ldflags, so the module version and VCS stamp in the build info fill in.
func buildVersion() (string, string) {
	v, sha := version, gitSha
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return v, sha
	}
	if v == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		v = info.Main.Version
	}
	if sha == "unknown" {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				sha = s.Value[:7]
			}
		}
	}
	return v, sha
}

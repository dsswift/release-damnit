// Package extrafile writes a new version into a package's release-please
// extra-files, changing only the version text and nothing around it.
package extrafile

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	inlineMarker = "x-release-please-version"
	blockStart   = "x-release-please-start-version"
	blockEnd     = "x-release-please-end"
)

// semverPattern matches the first version on a line, the same shape
// release-please's generic updater replaces.
var semverPattern = regexp.MustCompile(`\d+\.\d+\.\d+(?:-[0-9A-Za-z.]+)?(?:\+[-0-9A-Za-z.]+)?`)

// UpdateGeneric replaces the first semver on every line that carries an
// x-release-please-version marker, and on every line inside an
// x-release-please-start-version ... x-release-please-end block. It fails when
// the content has no marker or a marked line has no version to replace.
func UpdateGeneric(content, newVersion string) (string, error) {
	lines := strings.Split(content, "\n")
	replaced := 0
	inBlock := false
	blockLine := 0

	for i, line := range lines {
		switch {
		case strings.Contains(line, blockStart):
			// x-release-please-start-version contains no inline marker text,
			// so this case never shadows one.
			if inBlock {
				return "", fmt.Errorf("line %d: %s inside an open block (opened on line %d)", i+1, blockStart, blockLine)
			}
			inBlock = true
			blockLine = i + 1
		case inBlock:
			if semverPattern.MatchString(line) {
				lines[i] = replaceFirst(line, newVersion)
				replaced++
			}
			if strings.Contains(line, blockEnd) {
				inBlock = false
			}
		case strings.Contains(line, inlineMarker):
			if !semverPattern.MatchString(line) {
				return "", fmt.Errorf("line %d has %s but no version to replace", i+1, inlineMarker)
			}
			lines[i] = replaceFirst(line, newVersion)
			replaced++
		}
	}

	if inBlock {
		return "", fmt.Errorf("block opened on line %d has no %s", blockLine, blockEnd)
	}
	if replaced == 0 {
		return "", fmt.Errorf("no %s marker or %s block with a version", inlineMarker, blockStart)
	}
	return strings.Join(lines, "\n"), nil
}

func replaceFirst(line, newVersion string) string {
	loc := semverPattern.FindStringIndex(line)
	return line[:loc[0]] + newVersion + line[loc[1]:]
}

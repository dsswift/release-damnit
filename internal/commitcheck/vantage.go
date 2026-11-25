package commitcheck

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// VantageFile is the pointer file a repo keeps at its root to name where its
// program is written down. Only its work and roadmap kinds matter here.
const VantageFile = "vantage.yaml"

// Tracking is a repo's per-commit work tracking. When Kind is set, every
// commit names the item it is part of in a "Work:" trailer: an item of Kind,
// or of Roadmap when the repo names one.
type Tracking struct {
	Kind    string
	Roadmap string
}

// On reports whether the repo tracks work per commit.
func (t Tracking) On() bool { return t.Kind != "" }

// kinds are the item kinds a Work trailer may name.
func (t Tracking) kinds() []string {
	if t.Roadmap == "" || t.Roadmap == t.Kind {
		return []string{t.Kind}
	}
	return []string{t.Kind, t.Roadmap}
}

// Trailer returns the paragraph that names items in a commit message,
// starting with the blank line that sets it apart. Each item must be a key
// like KIND-15, and one of the repo's kinds when it tracks work.
func (t Tracking) Trailer(items []string) (string, error) {
	if len(items) == 0 {
		return "", errors.New("no work items given")
	}
	for _, item := range items {
		if p := t.itemProblem(item); p != "" {
			return "", errors.New(p)
		}
	}
	return "\n\nWork: " + strings.Join(items, ", "), nil
}

var kindPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)

// ReadTracking reads the work tracking from root's vantage.yaml. A repo with
// no such file, or one that names no work kind, does not track work.
func ReadTracking(root string) (Tracking, error) {
	f, err := os.Open(filepath.Join(root, VantageFile))
	if errors.Is(err, os.ErrNotExist) {
		return Tracking{}, nil
	}
	if err != nil {
		return Tracking{}, err
	}
	defer f.Close()

	var t Tracking
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		key, value, ok := topLevel(scanner.Text())
		if !ok || (key != "work" && key != "roadmap") || value == "" {
			continue
		}
		if !kindPattern.MatchString(value) {
			return Tracking{}, fmt.Errorf("%s: %s %q must be a kind key, letters then letters or digits", VantageFile, key, value)
		}
		if key == "work" {
			t.Kind = value
		} else {
			t.Roadmap = value
		}
	}
	if err := scanner.Err(); err != nil {
		return Tracking{}, fmt.Errorf("read %s: %w", VantageFile, err)
	}
	if !t.On() {
		return Tracking{}, nil
	}
	return t, nil
}

// topLevel splits a top-level "key: value" line, dropping a trailing
// comment and quotes around the value. Indented lines, comments, and lines
// that are not a key and a value report false.
func topLevel(line string) (key, value string, ok bool) {
	if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
		return "", "", false
	}
	key, value, ok = strings.Cut(line, ":")
	if !ok {
		return "", "", false
	}
	if i := strings.Index(value, " #"); i >= 0 {
		value = value[:i]
	}
	value = strings.TrimSpace(value)
	if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
		value = value[1 : len(value)-1]
	}
	return strings.TrimSpace(key), value, true
}

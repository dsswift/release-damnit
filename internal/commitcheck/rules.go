// Package commitcheck enforces the commit convention: a conventional header
// whose scope names the one package the commit touches, and, in a repo that
// tracks work, a trailer naming the item the commit is part of.
package commitcheck

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dsswift/release-damnit/internal/config"
)

// Rules is what a message is checked against.
type Rules struct {
	Types            []string
	Scopes           []string
	SubjectMaxLength int

	// Config maps files to packages. Nil when the repo has no
	// release-please config: then only the header rules apply and any scope
	// is accepted.
	Config *config.Config

	// Work is the repo's work tracking from its vantage.yaml. When it is
	// on, every message names its item in a "Work:" trailer.
	Work Tracking
}

// RulesFor builds the rules from a loaded config, or the defaults when cfg is nil.
func RulesFor(cfg *config.Config) Rules {
	if cfg == nil {
		return Rules{Types: config.DefaultTypes, SubjectMaxLength: config.DefaultSubjectMaxLength}
	}
	return Rules{Types: cfg.Types, Scopes: cfg.Scopes, SubjectMaxLength: cfg.SubjectMaxLength, Config: cfg}
}

// Header is a parsed conventional commit header: type(scope)!: description.
type Header struct {
	Type        string
	Scope       string
	HasScope    bool
	Breaking    bool
	Description string
}

var headerPattern = regexp.MustCompile(`^([A-Za-z]+)(\(([^()]*)\))?(!)?: (.*)$`)

// ParseHeader parses a header line. It reports false when the line is not in
// the type(scope)!: description shape at all.
func ParseHeader(line string) (Header, bool) {
	m := headerPattern.FindStringSubmatch(line)
	if m == nil {
		return Header{}, false
	}
	return Header{Type: m[1], HasScope: m[2] != "", Scope: m[3], Breaking: m[4] == "!", Description: m[5]}, true
}

// skipPrefixes mark messages git or the release tooling writes. They pass
// without a check.
var skipPrefixes = []string{"Merge ", `Revert "`, "fixup! ", "squash! ", "chore: release", "chore(release):"}

// Result is the outcome of checking one message.
type Result struct {
	// Header is the first line of the message.
	Header string

	// Skipped is true for merge, revert, fixup, squash and release commits.
	Skipped bool

	// Problems are the rule violations, one actionable line each. Empty
	// means the message passes.
	Problems []string
}

// OK reports whether the message passed.
func (r Result) OK() bool { return len(r.Problems) == 0 }

// Headline returns the header of a raw commit message: its first line once
// comment lines and leading blank lines are dropped.
func Headline(message string) string {
	for _, line := range strings.Split(message, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		return line
	}
	return ""
}

// Check checks a commit message against the rules, with files being every
// path the commit touches (both sides of a rename, and deletions).
func Check(rules Rules, message string, files []string) Result {
	header := Headline(message)
	result := Result{Header: header}

	if header == "" {
		result.Problems = []string{"the commit message is empty"}
		return result
	}
	for _, prefix := range skipPrefixes {
		if strings.HasPrefix(header, prefix) {
			result.Skipped = true
			return result
		}
	}

	h, ok := ParseHeader(header)
	if !ok {
		result.Problems = []string{fmt.Sprintf("header %q must look like type(scope): description, with one space after the colon", header)}
	} else {
		result.Problems = append(result.Problems, checkHeader(rules, h)...)
		result.Problems = append(result.Problems, checkScope(rules, h, files)...)
	}
	result.Problems = append(result.Problems, checkWork(rules.Work, message)...)
	return result
}

func checkHeader(rules Rules, h Header) []string {
	var problems []string

	if !contains(rules.Types, h.Type) {
		problems = append(problems, fmt.Sprintf("type %q is not allowed; use one of: %s", h.Type, strings.Join(rules.Types, ", ")))
	}
	if h.Breaking && h.Type != "feat" {
		problems = append(problems, fmt.Sprintf("the breaking marker ! is allowed only on feat; write feat!: or feat(scope)!:, not %s!", h.Type))
	}
	if h.HasScope && h.Scope == "" {
		problems = append(problems, "the scope is empty; drop the parentheses or name the scope")
	}

	desc := h.Description
	switch {
	case strings.TrimSpace(desc) == "":
		problems = append(problems, "the description after the colon is empty")
	case strings.HasPrefix(desc, " "):
		problems = append(problems, "use exactly one space after the colon")
	default:
		if first, _ := utf8.DecodeRuneInString(desc); unicode.IsUpper(first) {
			problems = append(problems, fmt.Sprintf("start the description with a lowercase letter: %q", desc))
		}
		if strings.HasSuffix(desc, ".") {
			problems = append(problems, "drop the period at the end of the description")
		}
		if n := utf8.RuneCountInString(desc); n > rules.SubjectMaxLength {
			problems = append(problems, fmt.Sprintf("the description is %d characters; the limit is %d", n, rules.SubjectMaxLength))
		}
	}
	return problems
}

func checkScope(rules Rules, h Header, files []string) []string {
	if rules.Config == nil {
		return nil
	}
	cfg := rules.Config
	single := len(cfg.Packages) == 1

	byPackage := map[*config.Package][]string{}
	for _, f := range files {
		if pkg := cfg.FindPackageForPath(f); pkg != nil {
			byPackage[pkg] = append(byPackage[pkg], f)
		}
	}

	switch len(byPackage) {
	case 0:
		if h.Scope == "" {
			if len(cfg.Packages) > 1 {
				return []string{scopeRequiredProblem(rules)}
			}
			return nil
		}
		if !contains(rules.Scopes, h.Scope) {
			return []string{fmt.Sprintf("scope %q is not a repo scope and this commit touches no package; %s", h.Scope, scopeChoices(rules))}
		}
		return nil

	case 1:
		for pkg, pkgFiles := range byPackage {
			if h.Scope == pkg.Scope() || (h.Scope == "" && single) {
				return nil
			}
			if h.Scope == "" {
				return []string{fmt.Sprintf("add the scope %q: this commit changes that package (%s)", pkg.Scope(), summarize(pkgFiles))}
			}
			return []string{fmt.Sprintf("scope %q does not match the package this commit changes; use %q (%s)", h.Scope, pkg.Scope(), summarize(pkgFiles))}
		}
	}

	packages := make([]*config.Package, 0, len(byPackage))
	for pkg := range byPackage {
		packages = append(packages, pkg)
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].Scope() < packages[j].Scope() })

	var b strings.Builder
	fmt.Fprintf(&b, "this commit changes %d packages; split into one commit per package:", len(packages))
	for _, pkg := range packages {
		fmt.Fprintf(&b, "\n  %s: %s", pkg.Scope(), strings.Join(byPackage[pkg], ", "))
	}
	return []string{b.String()}
}

var itemPattern = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9]*)-([0-9]+)$`)

// checkWork checks the "Work:" trailers in the message's trailer block, its
// last paragraph after the header. Each names one or more items, comma
// separated, of the repo's work or roadmap kind.
func checkWork(t Tracking, message string) []string {
	if !t.On() {
		return nil
	}
	paragraphs := bodyParagraphs(message)
	var trailers []string
	if len(paragraphs) > 1 {
		trailers = workValues(paragraphs[len(paragraphs)-1])
	}
	if len(trailers) == 0 {
		for i, p := range paragraphs {
			if i == 0 {
				p = p[1:]
			}
			if len(workValues(p)) > 0 {
				return []string{fmt.Sprintf("the \"Work:\" trailer must be in the last paragraph of the message, with any other trailers (%s)", VantageFile)}
			}
		}
		return []string{fmt.Sprintf("no \"Work:\" trailer; end the message with a line like \"Work: %s\" naming the %s item this commit is part of (%s)", t.example(), t.kindNames(), VantageFile)}
	}

	var problems []string
	for _, value := range trailers {
		items := strings.Split(value, ",")
		for _, item := range items {
			item = strings.TrimSpace(item)
			if p := t.itemProblem(item); p != "" {
				problems = append(problems, fmt.Sprintf("\"Work: %s\": %s", strings.TrimSpace(value), p))
			}
		}
	}
	return problems
}

// itemProblem says what is wrong with one item key, or nothing.
func (t Tracking) itemProblem(item string) string {
	m := itemPattern.FindStringSubmatch(item)
	if m == nil {
		return fmt.Sprintf("%q is not an item key like %q; name one or more, comma separated", item, t.example())
	}
	if t.On() && !contains(t.kinds(), m[1]) {
		return fmt.Sprintf("%s is of kind %s; name the %s item this commit is part of (%s)", item, m[1], t.kindNames(), VantageFile)
	}
	return ""
}

func (t Tracking) example() string {
	if !t.On() {
		return "KIND-15"
	}
	return t.Kind + "-15"
}

func (t Tracking) kindNames() string { return strings.Join(t.kinds(), " or ") }

// bodyParagraphs splits a raw message into its paragraphs, the header's
// first, as git keeps it: comment lines are dropped, and nothing below the
// scissors line of a verbose commit counts.
func bodyParagraphs(message string) [][]string {
	var paragraphs [][]string
	var current []string
	for _, line := range strings.Split(message, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "# ------------------------ >8 ------------------------") {
			break
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.TrimSpace(line) == "" {
			if len(current) > 0 {
				paragraphs = append(paragraphs, current)
				current = nil
			}
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		paragraphs = append(paragraphs, current)
	}
	return paragraphs
}

// workValues returns the value of each "Work:" line in a paragraph. The key
// matches in any case, as git trailer keys do.
func workValues(paragraph []string) []string {
	var values []string
	for _, line := range paragraph {
		key, value, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(key, "Work") {
			values = append(values, value)
		}
	}
	return values
}

func scopeRequiredProblem(rules Rules) string {
	return "this commit touches no package, so it needs a repo scope; " + scopeChoices(rules)
}

func scopeChoices(rules Rules) string {
	if len(rules.Scopes) == 0 {
		return "the config lists no repo scopes; add one to \"scopes\" in " + config.ConfigFile
	}
	return "use one of: " + strings.Join(rules.Scopes, ", ")
}

// summarize names up to three files, so a long list does not flood the hook output.
func summarize(files []string) string {
	if len(files) <= 3 {
		return strings.Join(files, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(files[:3], ", "), len(files)-3)
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

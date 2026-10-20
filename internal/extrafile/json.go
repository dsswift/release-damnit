package extrafile

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// pathSegment is one step of a JSONPath: an object key or an array index.
type pathSegment struct {
	key     string
	index   int
	isIndex bool
}

func (s pathSegment) String() string {
	if s.isIndex {
		return fmt.Sprintf("[%d]", s.index)
	}
	return "[" + strconv.Quote(s.key) + "]"
}

// UpdateJSON sets the string value at jsonPath to newVersion. Only the bytes
// of that string change; whitespace, key order and every other value stay
// exactly as they were. It fails when the content is not valid JSON, the path
// does not exist, or the value there is not a string.
func UpdateJSON(content, jsonPath, newVersion string) (string, error) {
	segments, err := parseJSONPath(jsonPath)
	if err != nil {
		return "", err
	}
	if !json.Valid([]byte(content)) {
		return "", fmt.Errorf("not valid JSON")
	}

	s := &scanner{src: content}
	start, end, err := s.find(segments)
	if err != nil {
		return "", fmt.Errorf("%s: %w", jsonPath, err)
	}
	encoded, err := json.Marshal(newVersion)
	if err != nil {
		return "", err
	}
	return content[:start] + string(encoded) + content[end:], nil
}

// parseJSONPath accepts $.a.b, $["a"].b, $['a'], $.list[0] and
// $.packages[""].version.
func parseJSONPath(path string) ([]pathSegment, error) {
	if !strings.HasPrefix(path, "$") {
		return nil, fmt.Errorf("jsonpath %q must start with $", path)
	}
	rest := path[1:]
	var segments []pathSegment
	for rest != "" {
		switch rest[0] {
		case '.':
			rest = rest[1:]
			n := strings.IndexAny(rest, ".[")
			if n == -1 {
				n = len(rest)
			}
			if n == 0 {
				return nil, fmt.Errorf("jsonpath %q has an empty key", path)
			}
			segments = append(segments, pathSegment{key: rest[:n]})
			rest = rest[n:]
		case '[':
			closeAt := strings.IndexByte(rest, ']')
			if len(rest) > 1 && (rest[1] == '"' || rest[1] == '\'') {
				quote := rest[1]
				endQuote := strings.IndexByte(rest[2:], quote)
				if endQuote == -1 || len(rest) < endQuote+4 || rest[endQuote+3] != ']' {
					return nil, fmt.Errorf("jsonpath %q has an unterminated quoted key", path)
				}
				segments = append(segments, pathSegment{key: rest[2 : 2+endQuote]})
				rest = rest[endQuote+4:]
				continue
			}
			if closeAt == -1 {
				return nil, fmt.Errorf("jsonpath %q has an unclosed [", path)
			}
			index, err := strconv.Atoi(rest[1:closeAt])
			if err != nil || index < 0 {
				return nil, fmt.Errorf("jsonpath %q: %q is not a quoted key or an array index", path, rest[1:closeAt])
			}
			segments = append(segments, pathSegment{index: index, isIndex: true})
			rest = rest[closeAt+1:]
		default:
			return nil, fmt.Errorf("jsonpath %q: unexpected %q", path, rest[0])
		}
	}
	if len(segments) == 0 {
		return nil, fmt.Errorf("jsonpath %q names the whole document, not a value", path)
	}
	return segments, nil
}

// scanner walks already-validated JSON text and reports byte offsets, so a
// value can be replaced in place.
type scanner struct {
	src string
	pos int
}

// find returns the span of the string value at segments, quotes included.
func (s *scanner) find(segments []pathSegment) (int, int, error) {
	for depth, seg := range segments {
		s.skipSpace()
		if err := s.enter(seg); err != nil {
			return 0, 0, fmt.Errorf("at %s: %w", describe(segments[:depth+1]), err)
		}
	}
	s.skipSpace()
	if s.peek() != '"' {
		return 0, 0, fmt.Errorf("value is not a string")
	}
	start := s.pos
	s.skipString()
	return start, s.pos, nil
}

// enter moves pos to the start of the value seg names inside the value at pos.
func (s *scanner) enter(seg pathSegment) error {
	if seg.isIndex {
		if s.peek() != '[' {
			return fmt.Errorf("not an array")
		}
		s.pos++
		for i := 0; ; i++ {
			s.skipSpace()
			if s.peek() == ']' {
				return fmt.Errorf("array has only %d elements", i)
			}
			if i == seg.index {
				return nil
			}
			s.skipValue()
			s.skipSpace()
			if s.peek() == ',' {
				s.pos++
			}
		}
	}

	if s.peek() != '{' {
		return fmt.Errorf("not an object")
	}
	s.pos++
	for {
		s.skipSpace()
		if s.peek() == '}' {
			return fmt.Errorf("key not found")
		}
		keyStart := s.pos
		s.skipString()
		var key string
		if err := json.Unmarshal([]byte(s.src[keyStart:s.pos]), &key); err != nil {
			return err
		}
		s.skipSpace()
		s.pos++ // ':'
		s.skipSpace()
		if key == seg.key {
			return nil
		}
		s.skipValue()
		s.skipSpace()
		if s.peek() == ',' {
			s.pos++
		}
	}
}

func (s *scanner) peek() byte {
	if s.pos >= len(s.src) {
		return 0
	}
	return s.src[s.pos]
}

func (s *scanner) skipSpace() {
	for s.pos < len(s.src) && strings.IndexByte(" \t\r\n", s.src[s.pos]) >= 0 {
		s.pos++
	}
}

func (s *scanner) skipString() {
	s.pos++ // opening quote
	for s.pos < len(s.src) {
		switch s.src[s.pos] {
		case '\\':
			s.pos += 2
		case '"':
			s.pos++
			return
		default:
			s.pos++
		}
	}
}

// skipValue moves past one value of any kind.
func (s *scanner) skipValue() {
	switch s.peek() {
	case '"':
		s.skipString()
	case '{', '[':
		depth := 0
		for s.pos < len(s.src) {
			switch s.src[s.pos] {
			case '"':
				s.skipString()
				continue
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
			s.pos++
			if depth == 0 {
				return
			}
		}
	default:
		for s.pos < len(s.src) && strings.IndexByte(",}] \t\r\n", s.src[s.pos]) < 0 {
			s.pos++
		}
	}
}

func describe(segments []pathSegment) string {
	var b strings.Builder
	b.WriteString("$")
	for _, seg := range segments {
		b.WriteString(seg.String())
	}
	return b.String()
}

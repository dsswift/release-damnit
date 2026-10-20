package extrafile

import (
	"strings"
	"testing"
)

func TestUpdateGeneric(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{
			name: "inline marker replaces only the first version on the line",
			in:   "const version = \"1.2.3\"; // x-release-please-version (was 1.0.0)\nconst other = \"9.9.9\"\n",
			want: "const version = \"2.0.0\"; // x-release-please-version (was 1.0.0)\nconst other = \"9.9.9\"\n",
		},
		{
			name: "block marks every line between start and end",
			in:   "a: 0.0.1\n# x-release-please-start-version\nimage: app:1.2.3\nappVersion: 1.2.3-rc.1\nname: no version here\n# x-release-please-end\nb: 0.0.1\n",
			want: "a: 0.0.1\n# x-release-please-start-version\nimage: app:2.0.0\nappVersion: 2.0.0\nname: no version here\n# x-release-please-end\nb: 0.0.1\n",
		},
		{
			name: "crlf line endings survive",
			in:   "v = \"1.0.0\" # x-release-please-version\r\nnext\r\n",
			want: "v = \"2.0.0\" # x-release-please-version\r\nnext\r\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := UpdateGeneric(tc.in, "2.0.0")
			if err != nil {
				t.Fatalf("UpdateGeneric: %v", err)
			}
			if got != tc.want {
				t.Errorf("got\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func TestUpdateGeneric_Failures(t *testing.T) {
	for name, in := range map[string]string{
		"no marker":              "version = 1.0.0\n",
		"marker without version": "// x-release-please-version\n",
		"unclosed block":         "# x-release-please-start-version\nv: 1.0.0\n",
		"block without version":  "# x-release-please-start-version\nnothing\n# x-release-please-end\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := UpdateGeneric(in, "2.0.0"); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestUpdateJSON_PreservesFormatting(t *testing.T) {
	in := "{\n    \"name\": \"web\",\n    \"version\":\"1.0.0\",\n\t\"nested\": { \"list\": [1, {\"v\": \"0.1.0\"}], \"version\": \"x\" },\n  \"packages\": {\n    \"\": {\n      \"name\": \"web\",\n      \"version\": \"1.0.0\"\n    },\n    \"node_modules/dep\": {\"version\": \"3.3.3\"}\n  }\n}\n"

	tests := []struct {
		path string
		want string
	}{
		{"$.version", strings.Replace(in, `"version":"1.0.0"`, `"version":"2.0.0"`, 1)},
		{`$.packages[""].version`, strings.Replace(in, "      \"version\": \"1.0.0\"", "      \"version\": \"2.0.0\"", 1)},
		{`$['packages']["node_modules/dep"].version`, strings.Replace(in, `"3.3.3"`, `"2.0.0"`, 1)},
		{"$.nested.list[1].v", strings.Replace(in, `"0.1.0"`, `"2.0.0"`, 1)},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got, err := UpdateJSON(in, tc.path, "2.0.0")
			if err != nil {
				t.Fatalf("UpdateJSON: %v", err)
			}
			if got != tc.want {
				t.Errorf("got\n%s\nwant\n%s", got, tc.want)
			}
		})
	}
}

func TestUpdateJSON_Failures(t *testing.T) {
	in := `{"version": 3, "a": {"b": "1.0.0"}, "list": []}`
	for _, path := range []string{
		"$.missing",
		"$.a.c",
		"$.version",
		"$.a.b.c",
		"$.list[0]",
		"version",
		"$",
		`$["unterminated]`,
	} {
		t.Run(path, func(t *testing.T) {
			if _, err := UpdateJSON(in, path, "2.0.0"); err == nil {
				t.Errorf("UpdateJSON(%s) succeeded, want an error", path)
			}
		})
	}
	if _, err := UpdateJSON(`{"version": "1.0.0",}`, "$.version", "2.0.0"); err == nil {
		t.Error("invalid JSON accepted")
	}
}

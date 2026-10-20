package config

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ExtraFileKind is how an extra file carries the version.
type ExtraFileKind string

const (
	// ExtraFileGeneric files mark version lines with x-release-please-version
	// or wrap them in x-release-please-start-version / x-release-please-end.
	ExtraFileGeneric ExtraFileKind = "generic"

	// ExtraFileJSON files hold the version as a string at a JSONPath.
	ExtraFileJSON ExtraFileKind = "json"
)

// ExtraFile is one entry of a package's release-please "extra-files" array.
type ExtraFile struct {
	Kind ExtraFileKind

	// Path is relative to the package path.
	Path string

	// JSONPath locates the version string; set only for ExtraFileJSON.
	JSONPath string
}

type extraFileObject struct {
	Type     ExtraFileKind `json:"type"`
	Path     string        `json:"path"`
	JSONPath string        `json:"jsonpath,omitempty"`
}

// UnmarshalJSON accepts release-please's two forms: a plain string (a generic
// file) or an object with type, path and, for json, jsonpath.
func (f *ExtraFile) UnmarshalJSON(data []byte) error {
	var path string
	if err := json.Unmarshal(data, &path); err == nil {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("extra-files: empty path")
		}
		*f = ExtraFile{Kind: ExtraFileGeneric, Path: path}
		return nil
	}

	var obj extraFileObject
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("extra-files: entry must be a path string or an object: %w", err)
	}
	if strings.TrimSpace(obj.Path) == "" {
		return fmt.Errorf("extra-files: entry %s has no path", data)
	}
	switch obj.Type {
	case ExtraFileGeneric:
		*f = ExtraFile{Kind: ExtraFileGeneric, Path: obj.Path}
	case ExtraFileJSON:
		if obj.JSONPath == "" {
			return fmt.Errorf("extra-files: json entry %s needs a jsonpath", obj.Path)
		}
		*f = ExtraFile{Kind: ExtraFileJSON, Path: obj.Path, JSONPath: obj.JSONPath}
	case "":
		return fmt.Errorf("extra-files: entry %s has no type", obj.Path)
	default:
		return fmt.Errorf("extra-files: type %q of %s is not supported (use generic or json)", obj.Type, obj.Path)
	}
	return nil
}

// MarshalJSON writes the object form, which states the type explicitly.
func (f ExtraFile) MarshalJSON() ([]byte, error) {
	return json.Marshal(extraFileObject{Type: f.Kind, Path: f.Path, JSONPath: f.JSONPath})
}

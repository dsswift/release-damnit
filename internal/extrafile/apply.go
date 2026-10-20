package extrafile

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dsswift/release-damnit/internal/config"
)

// Update writes newVersion into one extra file of pkg. The file must exist:
// a configured extra file that is missing is a config error, not something
// to create.
func Update(repoRoot string, pkg *config.Package, file config.ExtraFile, newVersion string) error {
	rel := pkg.RepoPath(file.Path)
	path := filepath.Join(repoRoot, filepath.FromSlash(rel))

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("extra file %s: %w", rel, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("extra file %s: %w", rel, err)
	}

	var updated string
	switch file.Kind {
	case config.ExtraFileGeneric:
		updated, err = UpdateGeneric(string(data), newVersion)
	case config.ExtraFileJSON:
		updated, err = UpdateJSON(string(data), file.JSONPath, newVersion)
	default:
		err = fmt.Errorf("unsupported extra-file type %q", file.Kind)
	}
	if err != nil {
		return fmt.Errorf("extra file %s: %w", rel, err)
	}

	return os.WriteFile(path, []byte(updated), info.Mode().Perm())
}

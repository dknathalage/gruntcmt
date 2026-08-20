package input

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dknathalage/gruntcmt/internal/plan"
)

// ReadPaths loads plan units from file and directory arguments. Directories are
// walked recursively for terragrunt's tfplan.json files; each unit's name is its
// location within the tree. A file that cannot be read or parsed becomes an
// isolated LoadError rather than failing the whole run.
func ReadPaths(paths []string) ([]plan.Unit, []plan.LoadError, error) {
	if len(paths) == 0 {
		return nil, nil, fmt.Errorf("no plan files given")
	}
	var units []plan.Unit
	var loadErrs []plan.LoadError
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, nil, fmt.Errorf("stat %s: %w", p, err)
		}
		if info.IsDir() {
			err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() || d.Name() != "tfplan.json" {
					return nil
				}
				addUnit(&units, &loadErrs, unitName(p, path), path)
				return nil
			})
			if err != nil {
				return nil, nil, fmt.Errorf("walk %s: %w", p, err)
			}
		} else {
			addUnit(&units, &loadErrs, unitName("", p), p)
		}
	}
	if len(units) == 0 && len(loadErrs) == 0 {
		return nil, nil, fmt.Errorf("no plans found")
	}
	return units, loadErrs, nil
}

func addUnit(units *[]plan.Unit, loadErrs *[]plan.LoadError, name, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		*loadErrs = append(*loadErrs, plan.LoadError{Name: name, Message: err.Error()})
		return
	}
	u, err := plan.ParsePlan(name, data)
	if err != nil {
		*loadErrs = append(*loadErrs, plan.LoadError{Name: name, Message: err.Error()})
		return
	}
	*units = append(*units, u)
}

// unitName derives a unit path. For a tfplan.json under a directory root, it is
// the parent directory relative to root. For an explicit file, it is the path
// with a .json suffix stripped (with the same tfplan.json → parent-dir rule).
func unitName(root, file string) string {
	if filepath.Base(file) == "tfplan.json" {
		dir := filepath.Dir(file)
		if root != "" {
			if rel, err := filepath.Rel(root, dir); err == nil {
				return filepath.ToSlash(rel)
			}
		}
		return filepath.ToSlash(dir)
	}
	return filepath.ToSlash(strings.TrimSuffix(filepath.Clean(file), ".json"))
}

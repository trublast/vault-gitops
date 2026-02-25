package gitops

import (
	"fmt"
	"strings"
)

func isObject(v interface{}) bool {
	if v == nil {
		return false
	}
	switch v.(type) {
	case map[string]interface{}, map[interface{}]interface{}:
		return true
	}
	return false
}

// Lint validates resources and returns an error on first failure.
func Lint(resources []Resource) error {
	byEffectiveName := make(map[string]int)
	for i := range resources {
		r := &resources[i]
		if r.Path == "" {
			return fmt.Errorf("resource at index %d: missing 'path'", i+1)
		}
		if r.Data == nil {
			return fmt.Errorf("resource at index %d (path %q): missing 'data'", i+1, r.Path)
		}
		if !isObject(r.Data) {
			return fmt.Errorf("resource at index %d (path %q): 'data' must be an object", i+1, r.Path)
		}
		if r.Revision < 0 {
			return fmt.Errorf("resource at index %d (path %q): revision must be non-negative (unsigned)", i+1, r.Path)
		}
		if m := strings.ToUpper(strings.TrimSpace(r.Method)); m != "" && m != "GET" && m != "POST" {
			return fmt.Errorf("resource at index %d (path %q): method must be GET or POST (got %q)", i+1, r.Path, r.Method)
		}
		eff := r.EffectiveName()
		if prev, exists := byEffectiveName[eff]; exists {
			return fmt.Errorf("duplicate name %q: resources at documents %d and %d", eff, prev+1, i+1)
		}
		byEffectiveName[eff] = i
	}

	for i := range resources {
		r := &resources[i]
		for j, depName := range r.Dependencies {
			if depName == "" {
				return fmt.Errorf("resource %q (doc %d): dependency %d: name must be non-empty", r.Path, i+1, j+1)
			}
			if _, exists := byEffectiveName[depName]; !exists {
				return fmt.Errorf("resource %q (doc %d): dependency %q not found", r.Path, i+1, depName)
			}
		}
	}
	return nil
}

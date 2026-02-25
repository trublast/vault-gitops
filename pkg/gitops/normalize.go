package gitops

import "strings"

func normalizeNamespace(s string) string {
	s = strings.TrimLeft(s, "/")
	if s == "" {
		return ""
	}
	if !strings.HasSuffix(s, "/") {
		s += "/"
	}
	return s
}

func normalizePath(s string) string {
	return strings.Trim(s, "/")
}

// NormalizeResource normalizes namespace and path on resource.
func NormalizeResource(r *Resource) {
	r.Namespace = normalizeNamespace(r.Namespace)
	r.Path = normalizePath(r.Path)
}

func normalizeValue(v interface{}) interface{} {
	switch x := v.(type) {
	case map[interface{}]interface{}:
		m := make(map[string]interface{})
		for k, val := range x {
			ks, _ := k.(string)
			m[ks] = normalizeValue(val)
		}
		return m
	case []interface{}:
		a := make([]interface{}, len(x))
		for i, val := range x {
			a[i] = normalizeValue(val)
		}
		return a
	default:
		return v
	}
}

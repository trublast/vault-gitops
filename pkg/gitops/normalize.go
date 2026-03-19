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
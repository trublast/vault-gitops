package gitops

// StateResource is stored per key (effective name: explicit name or namespace+path).
type StateResource struct {
	DataDigest     string      `json:"data_digest"`
	Dependencies   []string    `json:"dependencies"`
	IgnoreFailures bool        `json:"ignore_failures,omitempty"`
	ResponseData   interface{} `json:"response_data,omitempty"`
	Namespace      string      `json:"namespace,omitempty"`
	Path           string      `json:"path,omitempty"`
}

// State is persisted to storage.
type State struct {
	Resources map[string]StateResource `json:"resources"`
}

// Resource is one declarative resource from YAML.
type Resource struct {
	Path           string      `yaml:"path"`
	Data           interface{} `yaml:"data"`
	Namespace      string      `yaml:"namespace"`
	Name           string      `yaml:"name"`
	Revision       int         `yaml:"revision"` // optional; default 0; participates in digest (bump to force re-apply)
	Dependencies   []string    `yaml:"dependencies"`
	IgnoreFailures bool        `yaml:"ignore_failures"`
	Method         string      `yaml:"method"` // optional; "GET" or "POST" (default POST)
}

func (r Resource) NamespaceOrDefault() string {
	if r.Namespace != "" {
		return r.Namespace
	}
	return ""
}

// EffectiveName returns the unique name for the resource: Name if set, else namespace+path.
func (r Resource) EffectiveName() string {
	if r.Name != "" {
		return r.Name
	}
	ns := normalizeNamespace(r.NamespaceOrDefault())
	pathN := normalizePath(r.Path)
	if ns == "" {
		return pathN
	}
	return ns + pathN
}

// Key returns the state key: effective name (unique).
func (r Resource) Key() string {
	return r.EffectiveName()
}

// FindByNsPath returns the state key and resource for the given namespace and path, if any.
func (s *State) FindByNsPath(namespace, path string) (string, StateResource, bool) {
	if s == nil || s.Resources == nil {
		return "", StateResource{}, false
	}
	ns := normalizeNamespace(namespace)
	pathN := normalizePath(path)
	for k, v := range s.Resources {
		if normalizeNamespace(v.Namespace) == ns && normalizePath(v.Path) == pathN {
			return k, v, true
		}
	}
	return "", StateResource{}, false
}

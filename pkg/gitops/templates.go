package gitops

import (
	"fmt"
	"strings"
)

// ResolveTemplates replaces template strings <name:key> in data with values from state.
func ResolveTemplates(data interface{}, state *State) (interface{}, error) {
	switch x := data.(type) {
	case map[interface{}]interface{}:
		m := mapInterfaceKeysToStrings(x)
		return resolveTemplatesMap(m, state)
	case map[string]interface{}:
		return resolveTemplatesMap(x, state)
	case []interface{}:
		return resolveTemplatesSlice(x, state)
	case string:
		return resolveTemplateString(x, state)
	default:
		return data, nil
	}
}

func mapInterfaceKeysToStrings(x map[interface{}]interface{}) map[string]interface{} {
	m := make(map[string]interface{}, len(x))
	for k, v := range x {
		ks, _ := k.(string)
		m[ks] = v
	}
	return m
}

func resolveTemplatesMap(m map[string]interface{}, state *State) (map[string]interface{}, error) {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		res, err := ResolveTemplates(v, state)
		if err != nil {
			return nil, err
		}
		out[k] = res
	}
	return out, nil
}

func resolveTemplatesSlice(a []interface{}, state *State) ([]interface{}, error) {
	out := make([]interface{}, len(a))
	for i, v := range a {
		res, err := ResolveTemplates(v, state)
		if err != nil {
			return nil, err
		}
		out[i] = res
	}
	return out, nil
}

func resolveTemplateString(s string, state *State) (string, error) {
	if !strings.HasPrefix(s, "<") || !strings.HasSuffix(s, ">") || len(s) < 4 {
		return s, nil
	}
	inner := s[1 : len(s)-1]
	parts := strings.SplitN(inner, ":", 2)
	if len(parts) != 2 {
		return s, nil
	}
	name, key := parts[0], parts[1]
	if name == "" || key == "" {
		return s, nil
	}
	res, ok := state.Resources[name]
	if !ok {
		return "", fmt.Errorf("template %q: resource %q not in state", s, name)
	}
	val, ok := getResponseDataPath(res.ResponseData, key)
	if !ok {
		return "", fmt.Errorf("template %q: path %q not found in response_data of %q", s, key, name)
	}
	return fmt.Sprint(val), nil
}

func getResponseDataPath(rd interface{}, path string) (interface{}, bool) {
	if path == "" {
		return nil, false
	}
	segments := strings.Split(path, ".")
	v := rd
	for _, seg := range segments {
		if v == nil {
			return nil, false
		}
		var next interface{}
		var ok bool
		switch m := v.(type) {
		case map[string]interface{}:
			next, ok = m[seg]
		case map[interface{}]interface{}:
			next, ok = m[seg]
		case []interface{}:
			var i int
			if _, err := fmt.Sscanf(seg, "%d", &i); err != nil || i < 0 || i >= len(m) {
				return nil, false
			}
			next, ok = m[i], true
		default:
			return nil, false
		}
		if !ok {
			return nil, false
		}
		v = next
	}
	return v, true
}

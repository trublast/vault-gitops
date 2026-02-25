package gitops

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// StateWriter is called to persist state and errors during apply.
type StateWriter interface {
	SaveState(ctx context.Context, state *State) error
}

// Apply runs vault-gitops apply: resolve templates, POST create/update, DELETE removed, update state.
func Apply(ctx context.Context, resources []Resource, vaultAddr, token string, state *State, writer StateWriter) error {
	vaultAddr = strings.TrimSuffix(vaultAddr, "/")
	if vaultAddr == "" {
		return fmt.Errorf("vault address is required")
	}
	if token == "" {
		return fmt.Errorf("vault token is required")
	}
	if state == nil || state.Resources == nil {
		state = &State{Resources: make(map[string]StateResource)}
	}

	order, err := topologicalOrder(resources)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 60 * time.Second}
	currentKeys := make(map[string]bool)
	for _, r := range resources {
		currentKeys[r.Key()] = true
	}

	// Create or update
	for _, idx := range order {
		r := &resources[idx]
		key := r.Key()
		resolvedData, err := ResolveTemplates(r.Data, state)
		if err != nil {
			msg := fmt.Sprintf("resource %s%s: %v", r.Namespace, r.Path, err)
			if !r.IgnoreFailures {
				return fmt.Errorf("%s", msg)
			}
			continue
		}
		rev := revisionForDigest(r.Revision)
		if prev, inState := state.Resources[key]; inState && prev.DataDigest == dataDigestWithRevision(resolvedData, rev) {
			continue
		}
		// Maybe state exists under old key (hash) after user added name to resource.
		if _, inState := state.Resources[key]; !inState {
			if oldKey, prev, found := state.FindByNsPath(r.NamespaceOrDefault(), r.Path); found && prev.DataDigest == dataDigestWithRevision(resolvedData, rev) {
				state.Resources[key] = StateResource{
					DataDigest:     prev.DataDigest,
					Dependencies:   prev.Dependencies,
					IgnoreFailures: prev.IgnoreFailures,
					ResponseData:   prev.ResponseData,
					Namespace:      r.NamespaceOrDefault(),
					Path:           r.Path,
				}
				delete(state.Resources, oldKey)
				if writer != nil {
					if err := writer.SaveState(ctx, state); err != nil {
						msg := fmt.Sprintf("resource %s%s: save state (migrate key): %v", r.Namespace, r.Path, err)
						if !r.IgnoreFailures {
							return fmt.Errorf("%s", msg)
						}
						continue
					}
				}
				continue
			}
		}
		url := vaultAddr + "/v1/" + strings.TrimPrefix(r.Path, "/")
		method := normalizeMethod(r.Method)
		var reqBody []byte
		if method == http.MethodPost {
			body, err := dataToJSON(resolvedData)
			if err != nil {
				msg := fmt.Sprintf("resource %s%s: json encode: %v", r.Namespace, r.Path, err)
				if !r.IgnoreFailures {
					return fmt.Errorf("%s", msg)
				}
				continue
			}
			reqBody = []byte(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(reqBody))
		if err != nil {
			msg := fmt.Sprintf("resource %s%s: new request: %v", r.Namespace, r.Path, err)
			if !r.IgnoreFailures {
				return fmt.Errorf("%s", msg)
			}
			continue
		}
		if method == http.MethodPost && len(reqBody) > 0 {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("X-Vault-Request", "true")
		req.Header.Set("X-Vault-Token", token)
		if r.Namespace != "" {
			req.Header.Set("X-Vault-Namespace", r.Namespace)
		}

		resp, err := client.Do(req)
		if err != nil {
			msg := fmt.Sprintf("resource %s%s: request: %v", r.Namespace, r.Path, err)
			if !r.IgnoreFailures {
				return fmt.Errorf("%s", msg)
			}
			continue
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			var responseData interface{}
			if len(respBody) > 0 {
				var parsed map[string]interface{}
				if json.Unmarshal(respBody, &parsed) == nil {
					if d, ok := parsed["data"]; ok {
						responseData = normalizeValue(d)
					}
				}
			}
			state.Resources[key] = StateResource{
				DataDigest:     dataDigestWithRevision(resolvedData, rev),
				Dependencies:   r.Dependencies,
				IgnoreFailures: r.IgnoreFailures,
				ResponseData:   responseData,
				Namespace:      r.NamespaceOrDefault(),
				Path:           r.Path,
			}
			if writer != nil {
				if err := writer.SaveState(ctx, state); err != nil {
					msg := fmt.Sprintf("resource %s%s: save state: %v", r.Namespace, r.Path, err)
					if !r.IgnoreFailures {
						return fmt.Errorf("%s", msg)
					}
					continue
				}
			}
		} else {
			msg := fmt.Sprintf("resource %s%s: %s\n%s", r.Namespace, r.Path, resp.Status, strings.TrimSpace(string(respBody)))
			if !r.IgnoreFailures {
				return fmt.Errorf("%s", msg)
			}
		}
	}

	// Delete
	var toDelete []string
	for key := range state.Resources {
		if !currentKeys[key] {
			toDelete = append(toDelete, key)
		}
	}
	deleteOrder := deleteOrderFromState(state, toDelete)
	for _, key := range deleteOrder {
		res := state.Resources[key]
		ns, path := res.Namespace, res.Path
		ignoreFailures := res.IgnoreFailures
		url := vaultAddr + "/v1/" + strings.TrimPrefix(path, "/")
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
		if err != nil {
			msg := fmt.Sprintf("delete %s%s: new request: %v", ns, path, err)
			if !ignoreFailures {
				return fmt.Errorf("%s", msg)
			}
			continue
		}
		req.Header.Set("X-Vault-Request", "true")
		req.Header.Set("X-Vault-Token", token)
		if ns != "" {
			req.Header.Set("X-Vault-Namespace", ns)
		}

		resp, err := client.Do(req)
		if err != nil {
			msg := fmt.Sprintf("delete %s%s: request: %v", ns, path, err)
			if !ignoreFailures {
				return fmt.Errorf("%s", msg)
			}
			continue
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			// DELETE succeeded
			delete(state.Resources, key)
			if writer != nil {
				if err := writer.SaveState(ctx, state); err != nil {
					msg := fmt.Sprintf("delete %s%s: save state: %v", ns, path, err)
					if !ignoreFailures {
						return fmt.Errorf("%s", msg)
					}
					continue
				}
			}
		case resp.StatusCode == 405 || resp.StatusCode == 404:
			// 405 Method Not Allowed: path does not support DELETE; 404: already gone or path invalid.
			// Remove from state and continue without calling DELETE again.
			delete(state.Resources, key)
			if writer != nil {
				if err := writer.SaveState(ctx, state); err != nil {
					msg := fmt.Sprintf("delete %s%s: save state after %s: %v", ns, path, resp.Status, err)
					if !ignoreFailures {
						return fmt.Errorf("%s", msg)
					}
					continue
				}
			}
		default:
			msg := fmt.Sprintf("delete %s%s: %s\n%s", ns, path, resp.Status, strings.TrimSpace(string(respBody)))
			if !ignoreFailures {
				return fmt.Errorf("%s", msg)
			}
		}
	}

	return nil
}

func revisionForDigest(revision int) uint64 {
	if revision < 0 {
		return 0
	}
	return uint64(revision)
}

func normalizeMethod(m string) string {
	switch strings.ToUpper(strings.TrimSpace(m)) {
	case "GET":
		return http.MethodGet
	default:
		return http.MethodPost
	}
}

func dataDigestWithRevision(data interface{}, revision uint64) string {
	norm := normalizeValue(data)
	input := map[string]interface{}{"data": norm, "revision": revision}
	b, err := json.Marshal(input)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func dataToJSON(data interface{}) (string, error) {
	norm := normalizeValue(data)
	b, err := json.Marshal(norm)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func deleteOrderFromState(state *State, keys []string) []string {
	keySet := make(map[string]bool)
	for _, k := range keys {
		keySet[k] = true
	}
	inDegree := make(map[string]int)
	adj := make(map[string][]string)
	for _, k := range keys {
		inDegree[k] = 0
		adj[k] = nil
	}
	for _, k := range keys {
		res := state.Resources[k]
		for _, depName := range res.Dependencies {
			if keySet[depName] {
				adj[depName] = append(adj[depName], k)
				inDegree[k]++
			}
		}
	}
	var queue []string
	for k := range inDegree {
		if inDegree[k] == 0 {
			queue = append(queue, k)
		}
	}
	var order []string
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		order = append(order, u)
		for _, v := range adj[u] {
			inDegree[v]--
			if inDegree[v] == 0 {
				queue = append(queue, v)
			}
		}
	}
	return order
}

func topologicalOrder(resources []Resource) ([]int, error) {
	byName := make(map[string]int)
	for i := range resources {
		byName[resources[i].EffectiveName()] = i
	}
	inDegree := make([]int, len(resources))
	adj := make([][]int, len(resources))
	for i := range resources {
		r := &resources[i]
		for _, depName := range r.Dependencies {
			depIdx, ok := byName[depName]
			if !ok {
				continue
			}
			adj[depIdx] = append(adj[depIdx], i)
			inDegree[i]++
		}
	}
	var queue []int
	for i := range inDegree {
		if inDegree[i] == 0 {
			queue = append(queue, i)
		}
	}
	var order []int
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		order = append(order, u)
		for _, v := range adj[u] {
			inDegree[v]--
			if inDegree[v] == 0 {
				queue = append(queue, v)
			}
		}
	}
	if len(order) != len(resources) {
		return nil, fmt.Errorf("cycle in dependencies")
	}
	return order, nil
}

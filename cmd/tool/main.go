package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/vault/api"
	"github.com/trublast/vault-plugin-gitops/pkg/gitops"
)

// fileStateWriter saves state to a file on each SaveState.
type fileStateWriter struct {
	filename string
}

var projectVersion string

func (w fileStateWriter) SaveState(ctx context.Context, state *gitops.State) error {
	if state == nil || w.filename == "" {
		return nil
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(w.filename, data, 0600)
}

func loadStateFromFile(filename string) (*gitops.State, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return &gitops.State{Resources: make(map[string]gitops.StateResource)}, nil
		}
		return nil, err
	}
	var state gitops.State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state.Resources == nil {
		state.Resources = make(map[string]gitops.StateResource)
	}
	return &state, nil
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}
	cmd := strings.ToLower(os.Args[1])

	if cmd == "version" {
		fmt.Printf("gitops-tool %s\n", projectVersion)
		return
	}

	var err error

	switch cmd {
	case "lint":
		if len(os.Args) != 3 {
			printUsage()
			os.Exit(1)
		}
		err = runLint(os.Args[2])
	case "test":
		fs := flag.NewFlagSet("test", flag.ExitOnError)
		stateFile := fs.String("state", "", "load and save state to file")
		_ = fs.Parse(os.Args[2:])
		path := fs.Arg(0)
		if path == "" {
			printUsage()
			os.Exit(1)
		}
		err = runTest(path, *stateFile)
	default:

		fmt.Fprintf(os.Stderr, "unknown command %q; use lint, test, or version\n", cmd)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", cmd, err)
		os.Exit(1)
	}

	fmt.Printf("\033[32m✔\033[0m %s passed\n", cmd)
}

func runLint(path string) error {
	resources, err := gitops.LoadResourcesFromPath(path)
	if err != nil {
		return fmt.Errorf("load: %w", err)
	}
	return gitops.Lint(resources)
}

func runTest(path, stateFile string) error {
	resources, err := gitops.LoadResourcesFromPath(path)
	if err != nil {
		return fmt.Errorf("load: %w", err)
	}
	if err := gitops.Lint(resources); err != nil {
		return fmt.Errorf("lint: %w", err)
	}

	if strings.TrimSpace(os.Getenv("VAULT_TOKEN")) == "" {
		return fmt.Errorf("VAULT_TOKEN is not set")
	}

	cfg := api.DefaultConfig()
	if err := cfg.ReadEnvironment(); err != nil {
		return fmt.Errorf("vault config: %w", err)
	}
	vaultClient, err := api.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("vault client: %w", err)
	}

	var state *gitops.State
	if stateFile != "" {
		state, err = loadStateFromFile(stateFile)
		if err != nil {
			return fmt.Errorf("load state: %w", err)
		}
	} else {
		state = &gitops.State{Resources: make(map[string]gitops.StateResource)}
	}
	if state.Resources == nil {
		state.Resources = make(map[string]gitops.StateResource)
	}

	var writer gitops.StateWriter
	if stateFile != "" {
		writer = fileStateWriter{filename: stateFile}
	}
	if err := gitops.Apply(context.Background(), resources, vaultClient, state, writer); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	if writer != nil {
		if err := writer.SaveState(context.Background(), state); err != nil {
			return fmt.Errorf("save state: %w", err)
		}
	}
	return nil
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "usage: gitops-tool lint <path>")
	fmt.Fprintln(os.Stderr, "       gitops-tool test [-state <file>] <path>")
	fmt.Fprintln(os.Stderr, "       gitops-tool version")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  lint:    validate declarative YAML (path, data, names, dependencies).")
	fmt.Fprintln(os.Stderr, "  test:    run apply against Vault; requires VAULT_ADDR and VAULT_TOKEN.")
	fmt.Fprintln(os.Stderr, "           -state: optional file to load state from and save state to.")
	fmt.Fprintln(os.Stderr, "  version: print version and exit.")
	fmt.Fprintln(os.Stderr, "  path:    file (.yaml/.yml) or directory (recursively collects .yaml/.yml)")
}

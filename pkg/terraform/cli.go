package terraform

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/vault/sdk/logical"
	trdlGit "github.com/trublast/vault-plugin-gitops/pkg/git"
)

const (
	// StorageKeyTerraformState is the key for storing terraform state in storage
	StorageKeyTerraformState = "terraform_state"
)

// CLIConfig contains configuration for terraform CLI execution
type CLIConfig struct {
	VaultAddr      string
	VaultToken     string
	VaultNamespace string
	TfPath         string
	TfBinary       string
	Storage        logical.Storage
	Logger         hclog.Logger
}

// ApplyTerraformFromRepo extracts terraform files from git repository and applies them using Terraform CLI
func ApplyTerraformFromRepo(ctx context.Context, gitRepo *git.Repository, config CLIConfig) error {
	// Create temporary directory for terraform files
	tmpDir, err := os.MkdirTemp("", "vault-plugin-terraform-*")
	if err != nil {
		return fmt.Errorf("creating temporary directory: %w", err)
	}

	// Save state before cleanup (even if there was an error)
	defer func() {

		// Save state before removing directory
		statePath := filepath.Join(tmpDir, config.TfPath, "terraform.tfstate")
		if stateData, readErr := os.ReadFile(statePath); readErr == nil && len(stateData) > 0 {
			if saveErr := saveTerraformState(ctx, stateData, config); saveErr != nil {
				config.Logger.Warn(fmt.Sprintf("Failed to save terraform state: %v", saveErr))
			}
		}
		os.RemoveAll(tmpDir)
	}()

	config.Logger.Debug(fmt.Sprintf("Created temporary directory for terraform: %q", tmpDir))

	// Extract terraform files from repository
	tfFiles, err := extractTerraformFiles(gitRepo, tmpDir, config.TfPath, config.Logger)
	if err != nil {
		return fmt.Errorf("extracting terraform files: %w", err)
	}

	if len(tfFiles) == 0 {
		config.Logger.Info("No terraform files found in repository")
		return nil
	}
	tfDir := filepath.Join(tmpDir, config.TfPath)

	fileInfo, err := os.Stat(tfDir)
	if err != nil || !fileInfo.IsDir() {
		return fmt.Errorf("%q is not a directory", config.TfPath)
	}

	// Load terraform state from storage if it exists
	if err := loadTerraformState(ctx, tfDir, config); err != nil {
		return fmt.Errorf("loading terraform state: %w", err)
	}

	// Run terraform init
	if err := runTerraformInit(ctx, tfDir, config); err != nil {
		return fmt.Errorf("terraform init: %w", err)
	}

	// Run terraform plan
	if err := runTerraformPlan(ctx, tfDir, config); err != nil {
		return fmt.Errorf("terraform plan: %w", err)
	}

	// Run terraform apply
	if err := runTerraformApply(ctx, tfDir, config); err != nil {
		return fmt.Errorf("terraform apply: %w", err)
	}

	return nil
}

// extractTerraformFiles extracts .tf and .hcl files from git repository to temporary directory
func extractTerraformFiles(gitRepo *git.Repository, targetDir string, tfPath string, logger hclog.Logger) ([]string, error) {
	var tfFiles []string

	// Normalize tfPath for comparison
	normalizedTfPath := filepath.Clean(tfPath)
	if normalizedTfPath == "." {
		normalizedTfPath = ""
	}

	err := trdlGit.ForEachWorktreeFile(gitRepo, func(filePath, link string, fileReader io.Reader, info os.FileInfo) error {
		// Skip directories and symlinks
		if info.IsDir() || link != "" {
			return nil
		}

		// Normalize file path for comparison
		normalizedFilePath := filepath.Clean(filePath)

		// Filter files by tfPath: if tfPath is set, only extract files from that directory
		var targetPath string
		if normalizedTfPath != "" {
			// Check if file is in the tfPath directory
			// filePath should start with tfPath/ or be exactly tfPath
			if normalizedFilePath != normalizedTfPath && !strings.HasPrefix(normalizedFilePath, normalizedTfPath+string(filepath.Separator)) {
				return nil // Skip files outside tfPath
			}

			// Calculate relative path from tfPath for target directory structure
			relPath, err := filepath.Rel(normalizedTfPath, normalizedFilePath)
			if err != nil {
				return fmt.Errorf("calculating relative path for %q: %w", filePath, err)
			}
			// Create target file path: extract to targetDir/tfPath/relPath
			// This ensures files are in the correct location for terraform to work in targetDir/tfPath
			targetPath = filepath.Join(targetDir, normalizedTfPath, relPath)
		} else {
			// If tfPath is empty, extract files from root
			targetPath = filepath.Join(targetDir, filePath)
		}

		// Create directory structure
		if err := os.MkdirAll(filepath.Dir(targetPath), 0700); err != nil {
			return fmt.Errorf("creating directory for %q: %w", filePath, err)
		}

		// Create and write file
		targetFile, err := os.Create(targetPath)
		if err != nil {
			return fmt.Errorf("creating file %q: %w", targetPath, err)
		}
		defer targetFile.Close()

		if _, err := io.Copy(targetFile, fileReader); err != nil {
			return fmt.Errorf("copying file %q: %w", filePath, err)
		}

		tfFiles = append(tfFiles, targetPath)
		logger.Debug(fmt.Sprintf("Extracted terraform file: %q", filePath))
		return nil
	})

	if err != nil {
		return nil, err
	}

	logger.Info(fmt.Sprintf("Extracted %d terraform files", len(tfFiles)))
	return tfFiles, nil
}

// setupTerraformConfigFile checks for .terraformrc in workDir and sets TF_CLI_CONFIG_FILE env var if found
func setupTerraformConfigFile(workDir string, cmd *exec.Cmd) {
	tfConfig := os.Getenv("TF_CLI_CONFIG_FILE")
	// Env exists, use env value
	if tfConfig != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("TF_CLI_CONFIG_FILE=%s", tfConfig))
		return
	}
	terraformrcPath := filepath.Join(workDir, ".terraformrc")
	if _, err := os.Stat(terraformrcPath); err == nil {
		// File exists, set environment variable
		cmd.Env = append(cmd.Env, fmt.Sprintf("TF_CLI_CONFIG_FILE=%s", terraformrcPath))
	}
}

// runTerraformInit runs terraform init
func runTerraformInit(ctx context.Context, workDir string, config CLIConfig) error {
	tfBinary := getTfBinary(config)
	cmd := exec.CommandContext(ctx, tfBinary, "init", "-no-color", "-input=false")
	cmd.Dir = workDir
	cmd.Stdout = io.Discard

	// Copy existing environment variables
	cmd.Env = os.Environ()

	// Setup terraform config file if exists
	setupTerraformConfigFile(workDir, cmd)

	cmd.Env = append(cmd.Env, "TF_IN_AUTOMATION=true")

	// Capture stderr to get error details
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	config.Logger.Info("Running terraform init")
	if err := cmd.Run(); err != nil {
		stderr := strings.TrimSpace(stderrBuf.String())
		if stderr != "" {
			return fmt.Errorf("terraform init failed: %s", stderr)
		}
		return fmt.Errorf("terraform init failed: %w", err)
	}

	config.Logger.Info("Terraform init completed successfully")
	return nil
}

// runTerraformPlan runs terraform plan
func runTerraformPlan(ctx context.Context, workDir string, config CLIConfig) error {
	tfBinary := getTfBinary(config)
	cmd := exec.CommandContext(ctx, tfBinary, "plan", "-no-color", "-input=false", "-out=tfplan")
	cmd.Dir = workDir
	cmd.Stdout = io.Discard

	// Copy existing environment variables
	cmd.Env = os.Environ()
	if config.VaultAddr != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("VAULT_ADDR=%s", config.VaultAddr))
	}
	if config.VaultToken != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("VAULT_TOKEN=%s", config.VaultToken))
	}
	if config.VaultNamespace != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("VAULT_NAMESPACE=%s", config.VaultNamespace))
	}

	// Setup terraform config file if exists
	setupTerraformConfigFile(workDir, cmd)

	cmd.Env = append(cmd.Env, "TF_IN_AUTOMATION=true")

	// Capture stderr to get error details
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	config.Logger.Info("Running terraform plan")
	if err := cmd.Run(); err != nil {
		stderr := strings.TrimSpace(stderrBuf.String())
		if stderr != "" {
			return fmt.Errorf("terraform plan failed: %s", stderr)
		}
		return fmt.Errorf("terraform plan failed: %w", err)
	}

	config.Logger.Info("Terraform plan completed successfully")
	return nil
}

// runTerraformApply runs terraform apply with the plan file and returns the state
// State is returned even if apply failed, so it can be saved for debugging/recovery
func runTerraformApply(ctx context.Context, workDir string, config CLIConfig) error {
	tfBinary := getTfBinary(config)
	cmd := exec.CommandContext(ctx, tfBinary, "apply", "-no-color", "-input=false", "-auto-approve", "tfplan")
	cmd.Dir = workDir
	cmd.Stdout = io.Discard

	// Copy existing environment variables
	cmd.Env = os.Environ()
	if config.VaultAddr != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("VAULT_ADDR=%s", config.VaultAddr))
	}
	if config.VaultToken != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("VAULT_TOKEN=%s", config.VaultToken))
	}
	if config.VaultNamespace != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("VAULT_NAMESPACE=%s", config.VaultNamespace))
	}
	// Setup terraform config file if exists
	setupTerraformConfigFile(workDir, cmd)

	cmd.Env = append(cmd.Env, "TF_IN_AUTOMATION=true")

	// Capture stderr to get error details
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	config.Logger.Info("Running terraform apply")
	if err := cmd.Run(); err != nil {
		stderr := strings.TrimSpace(stderrBuf.String())
		if stderr != "" {
			return fmt.Errorf("terraform apply failed: %s", stderr)
		}
		return fmt.Errorf("terraform apply failed: %w", err)
	}

	config.Logger.Info("Terraform apply completed successfully")
	return nil
}

// loadTerraformState loads terraform state from storage and writes it to workDir
func loadTerraformState(ctx context.Context, workDir string, config CLIConfig) error {
	if config.Storage == nil {
		config.Logger.Debug("Storage not provided, skipping state load")
		return nil
	}

	entry, err := config.Storage.Get(ctx, StorageKeyTerraformState)
	if err != nil {
		return fmt.Errorf("getting terraform state from storage: %w", err)
	}

	if entry == nil || len(entry.Value) == 0 {
		config.Logger.Debug("No terraform state found in storage, starting fresh")
		return nil
	}

	// Write state file to workDir
	statePath := filepath.Join(workDir, "terraform.tfstate")
	if err := os.WriteFile(statePath, entry.Value, 0600); err != nil {
		return fmt.Errorf("writing terraform state file: %w", err)
	}

	config.Logger.Debug("Loaded terraform state from storage")
	return nil
}

// saveTerraformState saves terraform state to storage
func saveTerraformState(ctx context.Context, state []byte, config CLIConfig) error {
	if config.Storage == nil {
		config.Logger.Debug("Storage not provided, skipping state save")
		return nil
	}

	if len(state) == 0 {
		config.Logger.Debug("No terraform state to save")
		return nil
	}

	entry := &logical.StorageEntry{
		Key:   StorageKeyTerraformState,
		Value: state,
	}

	if err := config.Storage.Put(ctx, entry); err != nil {
		return fmt.Errorf("saving terraform state to storage: %w", err)
	}

	config.Logger.Info("Saved terraform state to storage")
	return nil
}

// getTfBinary returns the terraform binary path from config, or "terraform" as default
func getTfBinary(config CLIConfig) string {
	if config.TfBinary != "" {
		return config.TfBinary
	}
	return "terraform"
}

// validateTfBinary validates that the terraform binary exists and is executable
func validateTfBinary(tfBinary string) error {
	if tfBinary == "" {
		return fmt.Errorf("terraform binary path cannot be empty")
	}

	// Check if it's an absolute path
	if filepath.IsAbs(tfBinary) {
		// Check if file exists
		fileInfo, err := os.Stat(tfBinary)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("terraform binary not found at path %q", tfBinary)
			}
			return fmt.Errorf("unable to access terraform binary at %q: %w", tfBinary, err)
		}

		// Check if it's a regular file (not a directory)
		if fileInfo.IsDir() {
			return fmt.Errorf("terraform binary path %q is a directory, not a file", tfBinary)
		}

		// Check if file is executable
		mode := fileInfo.Mode()
		if mode&0111 == 0 {
			return fmt.Errorf("terraform binary at %q does not have execute permissions", tfBinary)
		}

		return nil
	}

	// If it's not an absolute path, check if it exists in PATH
	path, err := exec.LookPath(tfBinary)
	if err != nil {
		return fmt.Errorf("terraform binary %q not found in PATH", tfBinary)
	}

	// Verify the found binary is executable
	fileInfo, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("unable to access terraform binary at %q: %w", path, err)
	}

	mode := fileInfo.Mode()
	if mode&0111 == 0 {
		return fmt.Errorf("terraform binary at %q (found in PATH) does not have execute permissions", path)
	}

	return nil
}

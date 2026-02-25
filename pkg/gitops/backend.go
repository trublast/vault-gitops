package gitops

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/vault/sdk/framework"
	"github.com/hashicorp/vault/sdk/logical"
)

const (
	FieldNamePath = "path"

	StorageKeyConfiguration = "gitops_configuration"
	StorageKeyState         = "gitops_state"
)

// Configuration for gitops (path to YAML in repo).
type Configuration struct {
	Path string `structs:"path" json:"path,omitempty"`
}

type backend struct {
	baseBackend *framework.Backend
}

func (b *backend) Logger() hclog.Logger {
	return b.baseBackend.Logger()
}

// Paths returns API paths for configure/gitops.
func Paths(baseBackend *framework.Backend) []*framework.Path {
	b := &backend{baseBackend: baseBackend}
	return []*framework.Path{
		{
			Pattern: "^configure/gitops/?$",
			Fields: map[string]*framework.FieldSchema{
				FieldNamePath: {
					Type:        framework.TypeString,
					Default:     "",
					Description: "Path to YAML files in the repository (e.g. 'vault' or empty for root).",
					Required:    false,
				},
			},
			Operations: map[logical.Operation]framework.OperationHandler{
				logical.CreateOperation: &framework.PathOperation{
					Callback: b.pathConfigureCreateOrUpdate,
					Summary:  "Create gitops configuration.",
				},
				logical.UpdateOperation: &framework.PathOperation{
					Callback: b.pathConfigureCreateOrUpdate,
					Summary:  "Update gitops configuration.",
				},
				logical.ReadOperation: &framework.PathOperation{
					Callback: b.pathConfigureRead,
					Summary:  "Read gitops configuration.",
				},
				logical.DeleteOperation: &framework.PathOperation{
					Callback: b.pathConfigureDelete,
					Summary:  "Delete gitops configuration.",
				},
			},
			ExistenceCheck:  b.pathConfigExistenceCheck,
			HelpSynopsis:    "Configure path to declarative YAML in the git repository.",
			HelpDescription: "path: directory or file path in the repo containing .yaml/.yml (empty = root).",
		},
	}
}

func (b *backend) pathConfigExistenceCheck(ctx context.Context, req *logical.Request, _ *framework.FieldData) (bool, error) {
	out, err := req.Storage.Get(ctx, StorageKeyConfiguration)
	if err != nil {
		return false, err
	}
	return out != nil, nil
}

func (b *backend) pathConfigureCreateOrUpdate(ctx context.Context, req *logical.Request, fields *framework.FieldData) (*logical.Response, error) {
	var config Configuration
	if req.Operation == logical.UpdateOperation {
		existing, err := GetConfig(ctx, req.Storage)
		if err != nil {
			return logical.ErrorResponse("unable to get existing configuration: %s", err.Error()), nil
		}
		if existing == nil {
			return logical.ErrorResponse("configuration does not exist, use CREATE"), nil
		}
		config = *existing
	}
	if v, ok := fields.GetOk(FieldNamePath); ok {
		config.Path = v.(string)
	}
	if config.Path != "" {
		if filepath.Clean(config.Path) != config.Path || strings.Contains(config.Path, "..") {
			return logical.ErrorResponse("%q is invalid", FieldNamePath), nil
		}
	}
	entry, err := logical.StorageEntryJSON(StorageKeyConfiguration, &config)
	if err != nil {
		return nil, err
	}
	if err := req.Storage.Put(ctx, entry); err != nil {
		return nil, err
	}
	return nil, nil
}

func (b *backend) pathConfigureRead(ctx context.Context, req *logical.Request, _ *framework.FieldData) (*logical.Response, error) {
	config, err := GetConfig(ctx, req.Storage)
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, nil
	}
	return &logical.Response{Data: map[string]interface{}{
		FieldNamePath: config.Path,
	}}, nil
}

func (b *backend) pathConfigureDelete(ctx context.Context, req *logical.Request, _ *framework.FieldData) (*logical.Response, error) {
	if err := req.Storage.Delete(ctx, StorageKeyConfiguration); err != nil {
		return nil, err
	}
	return nil, nil
}

// GetConfig returns the gitops configuration from storage.
func GetConfig(ctx context.Context, storage logical.Storage) (*Configuration, error) {
	entry, err := storage.Get(ctx, StorageKeyConfiguration)
	if err != nil {
		return nil, fmt.Errorf("getting gitops config: %w", err)
	}
	if entry == nil || len(entry.Value) == 0 {
		return nil, nil
	}
	var c Configuration
	if err := json.Unmarshal(entry.Value, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

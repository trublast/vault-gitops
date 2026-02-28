package pgp

import (
	"bytes"
	"context"
	"fmt"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/hashicorp/vault/sdk/framework"
	"github.com/hashicorp/vault/sdk/logical"
)

const (
	fieldNameTrustedPGPPublicKeyName = "name"
	fieldNameTrustedPGPPublicKeyData = "public_key"
)

func Paths() []*framework.Path {
	return []*framework.Path{
		{
			Pattern:         "configure/trusted_pgp_public_key/?$",
			HelpSynopsis:    "List trusted PGP public keys",
			HelpDescription: "List all named trusted PGP public keys to check git repository commit signatures",
			Operations: map[logical.Operation]framework.OperationHandler{
				logical.ListOperation: &framework.PathOperation{
					Description: "Get the list of trusted PGP public keys",
					Callback:    pathConfigureTrustedPGPPublicKeyList,
				},
			},
		},
		{
			Pattern:         "configure/trusted_pgp_public_key/" + framework.GenericNameRegex(fieldNameTrustedPGPPublicKeyName) + "$",
			HelpSynopsis:    "CRUD operations for trusted PGP public key",
			HelpDescription: "Create, Read, Update, and Delete trusted PGP public key",
			Fields: map[string]*framework.FieldSchema{
				fieldNameTrustedPGPPublicKeyName: {
					Type:        framework.TypeNameString,
					Description: "Key name",
					Required:    true,
				},
				fieldNameTrustedPGPPublicKeyData: {
					Type:        framework.TypeString,
					Description: "Key data (required for CREATE/UPDATE)",
					Required:    false,
				},
			},
			Operations: map[logical.Operation]framework.OperationHandler{
				logical.CreateOperation: &framework.PathOperation{
					Description: "Add a trusted PGP public key",
					Callback:    pathConfigureTrustedPGPPublicKeyCreateOrUpdate,
				},
				logical.UpdateOperation: &framework.PathOperation{
					Description: "Update a trusted PGP public key",
					Callback:    pathConfigureTrustedPGPPublicKeyCreateOrUpdate,
				},
				logical.ReadOperation: &framework.PathOperation{
					Description: "Read the trusted PGP public key",
					Callback:    pathConfigureTrustedPGPPublicKeyRead,
				},
				logical.DeleteOperation: &framework.PathOperation{
					Description: "Delete the trusted PGP public key",
					Callback:    pathConfigureTrustedPGPPublicKeyDelete,
				},
			},
			ExistenceCheck: pathKeyExistenceCheck,
		},
	}
}

// pathKeyExistenceCheck verifies if the key exists.
func pathKeyExistenceCheck(ctx context.Context, req *logical.Request, fields *framework.FieldData) (bool, error) {
	name := fields.Get(fieldNameTrustedPGPPublicKeyName).(string)
	out, err := req.Storage.Get(ctx, trustedPGPPublicKeyStorageKey(name))
	if err != nil {
		return false, fmt.Errorf("existence check failed: %w", err)
	}

	return out != nil, nil
}

func pathConfigureTrustedPGPPublicKeyCreateOrUpdate(ctx context.Context, req *logical.Request, fields *framework.FieldData) (*logical.Response, error) {
	// Name is extracted from URL path via GenericNameRegex
	name := fields.Get(fieldNameTrustedPGPPublicKeyName).(string)
	if name == "" {
		return logical.ErrorResponse("key name is required"), nil
	}

	// Public key data is required for CREATE/UPDATE operations
	keyData, ok := fields.GetOk(fieldNameTrustedPGPPublicKeyData)
	if !ok {
		return logical.ErrorResponse("public_key field is required for CREATE/UPDATE operations"), nil
	}
	key := keyData.(string)
	if key == "" {
		return logical.ErrorResponse("public_key field cannot be empty"), nil
	}

	if err := IsValidGPGPublicKey(key); err != nil {
		return logical.ErrorResponse("invalid PGP public key: %v", err), nil
	}

	if err := req.Storage.Put(ctx, &logical.StorageEntry{
		Key:   trustedPGPPublicKeyStorageKey(name),
		Value: []byte(key),
	}); err != nil {
		return nil, fmt.Errorf("unable to put trusted pgp public key: %w", err)
	}

	return nil, nil
}

func pathConfigureTrustedPGPPublicKeyList(ctx context.Context, req *logical.Request, _ *framework.FieldData) (*logical.Response, error) {
	list, err := req.Storage.List(ctx, storageKeyPrefixTrustedPGPPublicKey)
	if err != nil {
		return nil, fmt.Errorf("unable to list %q in storage: %w", storageKeyPrefixTrustedPGPPublicKey, err)
	}

	return logical.ListResponse(list), nil
}

func pathConfigureTrustedPGPPublicKeyRead(ctx context.Context, req *logical.Request, fields *framework.FieldData) (*logical.Response, error) {
	name := fields.Get(fieldNameTrustedPGPPublicKeyName).(string)

	e, err := req.Storage.Get(ctx, trustedPGPPublicKeyStorageKey(name))
	if err != nil {
		return nil, err
	}

	if e == nil {
		return logical.ErrorResponse("PGP public key %q not found in storage", name), nil
	}

	return &logical.Response{
		Data: map[string]interface{}{
			"name":       name,
			"public_key": string(e.Value),
		},
	}, nil
}

func pathConfigureTrustedPGPPublicKeyDelete(ctx context.Context, req *logical.Request, fields *framework.FieldData) (*logical.Response, error) {
	name := fields.Get(fieldNameTrustedPGPPublicKeyName).(string)
	if err := req.Storage.Delete(ctx, trustedPGPPublicKeyStorageKey(name)); err != nil {
		return nil, err
	}

	return nil, nil
}

func IsValidGPGPublicKey(key string) error {
	reader := bytes.NewReader([]byte(key))

	entityList, err := openpgp.ReadArmoredKeyRing(reader)
	if err != nil {
		return fmt.Errorf("failed to parse key: %w", err)
	}

	if len(entityList) == 0 {
		return fmt.Errorf("no public key found in the input")
	}

	for _, entity := range entityList {
		if entity.PrimaryKey != nil {
			return nil
		}
	}

	return fmt.Errorf("no valid public key found")
}

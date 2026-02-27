package gitops

import (
	"context"

	"github.com/hashicorp/vault/sdk/logical"

	"github.com/trublast/vault-plugin-gitops/pkg/util"
)

// StorageStateWriter persists gitops state to logical.Storage.
// Used by the plugin in gitops mode; not used in terraform mode.
type StorageStateWriter struct {
	Storage logical.Storage
}

// SaveState implements StateWriter.
func (w *StorageStateWriter) SaveState(ctx context.Context, state *State) error {
	if state == nil {
		return nil
	}
	return util.PutJSON(ctx, w.Storage, StorageKeyState, state)
}

// NewStorageStateWriter returns a StateWriter that saves state to the given logical.Storage.
func NewStorageStateWriter(storage logical.Storage) StateWriter {
	return &StorageStateWriter{Storage: storage}
}

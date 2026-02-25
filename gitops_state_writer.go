package plugin_gitops

import (
	"context"

	"github.com/hashicorp/vault/sdk/logical"

	"github.com/trublast/vault-plugin-gitops/pkg/gitops"
	"github.com/trublast/vault-plugin-gitops/pkg/util"
)

// storageStateWriter persists gitops state and error to logical.Storage.
type storageStateWriter struct {
	storage logical.Storage
}

func (w *storageStateWriter) SaveState(ctx context.Context, state *gitops.State) error {
	if state == nil {
		return nil
	}
	return util.PutJSON(ctx, w.storage, gitops.StorageKeyState, state)
}

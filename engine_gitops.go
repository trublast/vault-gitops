//go:build !no_gitops

package plugin_gitops

import _ "github.com/trublast/vault-plugin-gitops/pkg/gitops" // registers gitops engine via init()

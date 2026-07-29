//go:build linux && enable_terraform

package plugin_gitops

import _ "github.com/trublast/vault-plugin-gitops/pkg/terraform" // registers terraform engine via init()

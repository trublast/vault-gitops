# gitops plugin self-manage policy

# allow to manage git repository configuration
path "gitops/configure/git_repository" {
    capabilities = ["read", "update"]
}

# allow to manage git credentials
path "gitops/configure/git_credential" {
    capabilities = ["read", "create" ,"update", "delete"]
}

# allow to manage trusted PGP public keys
path "gitops/configure/trusted_pgp_public_key/+" {
    capabilities = ["read", "create" ,"update", "delete"]
}

# allow to manage the plugin's terraform configuration
path "gitops/configure/terraform" {
    capabilities = ["read", "create" ,"update", "delete"]
}

# allow creating a new short-lived token for terraform to apply the configuration
# Terraform creates a new token for each apply operation
# This can be overridden by the TERRAFORM_VAULT_SKIP_CHILD_TOKEN environment variable
path "auth/token/create" {
  capabilities = ["update"]
}

# allow token rotation
path "auth/token/create-orphan" {
  capabilities = ["create"]
}

# allow to manage own policy
path "sys/policies/acl/self-manage-gitops" {
  capabilities = ["read", "update"]
}

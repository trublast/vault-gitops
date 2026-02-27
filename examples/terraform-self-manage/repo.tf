resource "vault_generic_secret" "repo" {
  path = "gitops/configure/git_repository"
  data_json = jsonencode({
    git_repo_url = "https://github.com/trublast/vault-plugin-gitops-terraform.git"
    required_number_of_verified_signatures_on_commit = "0"
    git_poll_period = "10m"
  })
}

resource "vault_generic_secret" "tf_path" {
  path = "gitops/configure/terraform"
  data_json = jsonencode({
    terraform_path = "examples/self-manage"
  })
}

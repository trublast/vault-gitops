resource "vault_policy" "self-manage-gitops" {
  name   = "self-manage-gitops"
  policy = file("self-manage-gitops.hcl")
}

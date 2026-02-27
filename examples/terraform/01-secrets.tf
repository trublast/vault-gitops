resource "vault_mount" "secret" {
  path        = "secret"
  type        = "kv"
  options     = { version = "2" }
  description = "KV Version 2 secret engine mount"
}

resource "vault_kv_secret_v2" "myapp_secret" {
  mount               = vault_mount.secret.path
  name                = "myapp"
  cas                 = 1
  delete_all_versions = true
  data_json = jsonencode(
    {
      password = "secret-password"
    }
  )
}

resource "vault_kv_secret_v2" "common_secret" {
  mount               = vault_mount.secret.path
  name                = "common"
  cas                 = 1
  delete_all_versions = true
  data_json = jsonencode(
    {
      password = "secret-password-common"
      user     = "user-common"
    }
  )
}

resource "vault_policy" "myapp" {
  name   = "myapp"
  policy = file("policies/myapp.hcl")
}

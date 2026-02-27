resource "vault_mount" "ssh_backend" {
  type = "ssh"
  path = "ssh"
}

resource "vault_ssh_secret_backend_ca" "ssh_ca" {
  backend              = vault_mount.ssh_backend.path
  generate_signing_key = true
}

resource "vault_ssh_secret_backend_role" "ssh_access" {
  name                    = "ssh_access"
  backend                 = vault_mount.ssh_backend.path
  key_type                = "ca"
  allow_user_certificates = true
  allowed_extensions      = "permit-pty,permit-port-forwarding"
  default_extensions      = { permit-pty = "" }
  allowed_users           = "*"
  ttl                     = "1800"
}

resource "vault_policy" "ssh_access" {
  name   = "ssh_access"
  policy = file("policies/ssh.hcl")
}

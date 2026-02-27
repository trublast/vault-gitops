path "database/creds/myapp" {
  capabilities = [ "read" ]
}
path "database/roles/*" {
  capabilities = [ "list" ]
}

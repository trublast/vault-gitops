resource "vault_namespace" "namespace_ns1" {
  path = "ns1"
}

resource "vault_namespace" "namespace_ns2" {
  path = "ns2"
}

resource "vault_namespace" "namespace_ns1_a1" {
  namespace  = vault_namespace.namespace_ns1.path_fq
  depends_on = [vault_namespace.namespace_ns1]
  path       = "a1"
}

resource "vault_namespace" "namespace_ns1_a2" {
  namespace  = vault_namespace.namespace_ns1.path_fq
  depends_on = [vault_namespace.namespace_ns1]
  path       = "a2"
}

resource "vault_namespace" "namespace_ns1_a1_b1" {
  namespace  = vault_namespace.namespace_ns1_a1.path_fq
  depends_on = [vault_namespace.namespace_ns1_a1]
  path       = "b1"
}

resource "vault_namespace" "namespace_ns2_a1" {
  namespace  = vault_namespace.namespace_ns2.path_fq
  depends_on = [vault_namespace.namespace_ns2]
  path       = "a1"
}

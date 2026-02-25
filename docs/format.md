# Declarative resource format (create/update)

Format for describing resources created or updated via the API from an OpenAPI specification. Only **create** and **update** operations (POST/PUT with request body) are considered, not list/get.

## Relation to OpenAPI and vault-client-go

- **OpenAPI**: paths like `/sys/mounts/{path}`, `/sys/policies/acl/{name}`; the operation has `requestBody` → `$ref` to a schema (e.g. `MountsEnableSecretsEngineRequest`, `PoliciesWriteAclPolicyRequest`).
- **vault-client-go**: for each such operation there is a method `OperationName(ctx, pathParam1, pathParam2, ..., requestBody)` — path params and body match the OpenAPI spec.

The declarative document specifies the **path with params already filled in** and the request **body** `data`, compatible with that operation’s schema.

---

## Minimal format (single resource)

```yaml
# Required
path: <path with path params filled in>
data: <object — request body per OpenAPI schema>

# Optional
namespace: /  # Vault namespace; if omitted, header is not sent
name: ""      # arbitrary resource name; must be unique (see below)
revision: 0   # non-negative integer; used in digest (increase to force re-apply)
dependencies: []  # list of resource names (name or namespace+path) this resource depends on (see below)
ignore_failures: false  # if true, apply error for this resource does not abort the whole apply
method: POST     # HTTP method: GET or POST (default POST); GET sends no body
```

- **path** — path without the `/v1/` prefix (client adds it). Path params from OpenAPI are already substituted, e.g.:
  - `sys/policies/acl/{name}` → `sys/policies/acl/mypolicy`
  - `sys/mounts/{path}` → `sys/mounts/mykv`
- **CRUD and resource deletion**: for correct DELETE when a resource is removed from the YAML, use paths where the resource identifier is part of the path (same as for read/update/delete in OpenAPI). For example, instead of `identity/group` (POST creates, but delete uses id) use `identity/group/name/my-group` (POST create/update, DELETE on the same path). Similarly: `identity/entity/name/my-entity`, `sys/policies/acl/mypolicy`, `sys/namespaces/ns1`, `auth/ldap/groups/dev-group`, `auth/ldap/users/ldap-user`. Some Vault API resources cannot be deleted on the same path they were created on, e.g. `pki/root/generate/internal`.
- **data** — JSON object for the request body. Keys and types follow OpenAPI (snake_case for Vault), matching the `requestBody` schema of the operation that matches (method, path).
- **namespace** — Vault namespace for the request (header `X-Vault-Namespace`). Set when the resource is not in root, e.g. `ns1/` or `ns1/team/`. Root resources have no `namespace` field.
- **name** — optional human-readable resource name. **State key** is always a unique name: if `name` is set, it is the key; otherwise the key is **namespace + path**. Normalized namespace with trailing `/` (e.g. `ns1/`) is concatenated with path: `ns=ns1`, `path=kv-v2/secret` → name `ns1/kv-v2/secret`; for root namespace only the path, e.g. `kv-v2/secret`. The linter checks name uniqueness. If only `name` changes (data unchanged), no API request is sent — only state is updated.
- **revision** — optional non-negative integer, default 0. Used in digest: with unchanged `data` the resource is not re-applied, but increasing `revision` (e.g. 0 to 1) changes the digest and the resource is applied again. Use for forced re-creation (resource deleted manually, cert/password regeneration, etc.). The linter checks that the value is non-negative.
- **dependencies** — optional list of resource **names** (explicit `name` or default namespace+path). This resource is applied after all of them. Format: list of strings, e.g. `[name1, name2]`. Apply and delete order is derived from the dependency graph (topological sort).
- **method** — optional HTTP method: `GET` or `POST` (default `POST`). For `GET` the request is sent with no body; use for read-only or list-style endpoints that accept GET. For `POST` the `data` object is sent as JSON in the request body.

---

## Namespaces

**Creating namespaces** — path `sys/namespaces/{path}` (CRUD: POST/DELETE). The path is the namespace name or nested path (`ns1`, `ns1/team`). POST body is schema `NamespacesCreateNamespaceRequest` (optional `custom_metadata`), can be `data: {}`.

```yaml
---
path: sys/namespaces/ns1
data: {}
---
path: sys/namespaces/ns1/team
data: {}
```

**Resources in namespaces**: set the **namespace** field on the document (e.g. `ns1/` or `ns1/team/`). The API request is sent with header `X-Vault-Namespace: ns1/` — create/update happens in that namespace. When a resource is removed from config, DELETE is also called with the same namespace.

**Apply order** is determined only by the **dependencies** graph: topological sort — a resource is created after all it depends on. There is no automatic ordering: e.g. for a namespace to be created before resources inside it, those resources must list the namespace resource in `dependencies` (e.g. `sys/namespaces/ns1`). Without dependencies, order follows the resource list order.

---

## Templates `<name:key>`

In **data** fields you can use placeholders from API responses of already-applied resources. A string **`<name:key>`** is replaced at apply time with a value from state: from the saved response (response_data) of the resource with the given **name** (`name`), the field at the dot path `key` is taken.

- **name** — name of the source resource (explicit `name` from config or default: namespace+path for a resource without a name).
- **key** — JSON path into that resource’s response: nested fields with dots, array elements by index. Examples: `client_token`, `keys.0`, `id`.

The template is only recognized if the string is **exactly** wrapped in angle brackets and has exactly two parts separated by `:`. Otherwise the string is left unchanged.

**Example**: a policy references `client_token` issued by a resource named `token-create`:

```yaml
---
name: token-create
path: auth/token/create
data:
  policies: ["default"]
  ttl: 1h
---
name: save-token-to-kv
path: kv1/mysecret
dependencies:
  - token-create
data:
  token: <token-create:client_token>
```

**Important**: a resource that uses a template must **depend** on the resource it references (via `dependencies` by name), otherwise apply order is not guaranteed and the substitution may not find the resource in state.

---

## ignore_failures

If **`ignore_failures: true`** is set, then on apply or delete error for this resource (template substitution error, invalid API response, state save error, etc.) the apply **does not stop**: the error is recorded but other resources continue. By default any resource error aborts the whole apply.

Useful for optional or environment-dependent resources (e.g. database connection when DB is unavailable) that should not block the rest.

Resources without a DELETE method in the API (e.g. `pki/root/generate/internal`): when removed from config, DELETE is called; if Vault returns 405 (Method Not Allowed) or 404, the entry is simply removed from state without error.

---

## Multiple resources (multi-document YAML)

A file can contain multiple documents separated by `---`; each document is one resource (one create/update).

```yaml
---
path: sys/policies/acl/mypolicy
data:
  policy: |
    path "*" { capabilities = ["read"] }
---
path: sys/mounts/mykv
namespace: ns1/
data:
  type: kv
  description: ""
  config:
    default_lease_ttl: 0s
    max_lease_ttl: 0s
  local: false
  seal_wrap: false
  external_entropy_access: false
```

**Example with dependencies** (resource names are used):

```yaml
---
name: ns1
path: sys/namespaces/ns1
data: {}
---
name: mykv
path: sys/mounts/mykv
namespace: ns1/
dependencies:
  - ns1
data:
  type: kv
  ...
---
name: ldap-config
path: auth/ldap/config
namespace: ns1/
dependencies:
  - ldap-mount
data:
  url: ldap://...
---
name: ldap-mount
path: sys/auth/ldap
namespace: ns1/
dependencies:
  - ns1
data: {}
```

---

## Summary

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `path` | yes | — | Path with path params filled in (no `/v1/`) |
| `data` | yes | — | Request body per OpenAPI schema for that operation |
| `namespace` | no | not sent | Vault namespace for the request (X-Vault-Namespace); resources in a namespace set e.g. `ns1/` or `ns1/team/` |
| `name` | no | "" | Resource name (state key); if unset — namespace+path (e.g. ns/kv/mysecret or kv/mysecret in root); must be unique |
| `revision` | no | 0 | Non-negative integer; used in digest (increase to force re-apply) |
| `dependencies` | no | [] | List of resource names; apply and delete order from dependency graph |
| `ignore_failures` | no | false | If true, apply error for this resource does not abort apply |
| `method` | no | POST | HTTP method: GET or POST; GET sends no body |

Minimum for one resource: **path** + **data**. Everything else is optional.

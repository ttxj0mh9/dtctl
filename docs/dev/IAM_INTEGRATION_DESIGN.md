# IAM Integration Design

**Status:** Design Proposal
**Created:** 2026-04-10
**Author:** dtctl team
**Reference:** [timstewart-dynatrace/dtiam](https://github.com/timstewart-dynatrace/dtiam) prototype

## Overview

This document proposes folding Dynatrace Identity and Access Management (IAM)
functionality into dtctl as a first-class subcommand tree: `dtctl iam`. The
design draws from the `dtiam` prototype, adapts it to dtctl's architecture,
and addresses the fundamental challenge that IAM operates on a different API
plane than the rest of dtctl.

## Goals

1. **Single binary** -- users manage environments _and_ IAM from one tool
2. **Unified config** -- one config file, one context concept, no second tool to configure
3. **Consistent UX** -- same verb-noun pattern, same output formats, same `--agent` mode
4. **Automation-ready** -- client-credentials flow for CI/CD alongside interactive PKCE
5. **Incremental adoption** -- IAM features are additive; existing commands unchanged

## Non-Goals

- Replacing the Dynatrace web UI for IAM administration
- Supporting Dynatrace Classic (non-Platform) IAM APIs
- Implementing dtiam's template engine (dtctl's `apply -f` covers this)
- Breaking existing config files or commands

---

## Background: Two API Planes

dtctl today operates on the **environment plane** -- every request targets a
specific tenant URL like `https://abc123.apps.dynatrace.com`. IAM operates on
the **account management plane** at `https://api.dynatrace.com`, scoped by an
account UUID that owns one or more environments.

| Dimension | Environment plane (dtctl today) | Account plane (IAM) |
|-----------|-------------------------------|---------------------|
| Base URL | `https://{envID}.apps.dynatrace.com` | `https://api.dynatrace.com` |
| Identity anchor | Environment ID | Account UUID |
| Auth scopes | `iam:users:read`, `iam:groups:read`, ... | `account-idm-read`, `account-idm-write`, `iam-policies-management`, ... |
| Token endpoint | `sso.dynatrace.com` | `sso.dynatrace.com` (same provider, different scopes) |
| Existing dtctl support | Full | Read-only users/groups via environment-scoped endpoints |

The core architectural decision is how to bridge these two planes within a
single tool and config system.

---

## Decision 1: Command Structure

### Chosen: `dtctl iam <verb> <resource>` subcommand tree

```
dtctl iam get users
dtctl iam get groups
dtctl iam get policies [--level account|global]
dtctl iam get bindings [--group NAME]
dtctl iam get boundaries
dtctl iam get environments
dtctl iam get service-users

dtctl iam describe user <email-or-uid>
dtctl iam describe group <name-or-uuid>
dtctl iam describe policy <name-or-uuid>
dtctl iam describe boundary <name-or-uuid>

dtctl iam create group --name "Team" [--description "..."]
dtctl iam create policy --name "viewer" --statement "ALLOW ..."
dtctl iam create binding --group "Team" --policy "viewer" [--boundary "prod"]
dtctl iam create boundary --name "prod" --zones "Production,Staging"
dtctl iam create service-user --name "CI Pipeline"

dtctl iam delete group <name-or-uuid>
dtctl iam delete policy <name-or-uuid>
dtctl iam delete binding --group <g> --policy <p>
dtctl iam delete user <email-or-uid>
dtctl iam delete service-user <name-or-uuid>

dtctl iam analyze user-permissions <email>
dtctl iam analyze permissions-matrix
dtctl iam analyze least-privilege

dtctl iam bulk add-users-to-group -f users.csv
dtctl iam bulk create-groups -f groups.yaml
dtctl iam bulk create-bindings -f bindings.yaml

dtctl iam whoami
```

### Rationale

- **Namespace isolation.** IAM resources (users, groups, policies) are
  account-level concepts. Putting them under `iam` avoids ambiguity with
  potential environment-level resources of the same name.
- **Auth boundary.** All `dtctl iam` commands know they need account-level
  auth and an `account-uuid`. The subcommand is a natural validation gate.
- **Discoverability.** `dtctl iam --help` lists all IAM operations in one
  place. Users don't need to guess which top-level verbs support IAM resources.
- **Industry precedent.** AWS CLI: `aws iam`. Google Cloud: `gcloud iam`.
  Azure: `az ad` / `az role`. All major cloud CLIs use a dedicated IAM
  namespace within the same binary.

### Alternatives Considered

**A. Top-level resources (`dtctl get iam-users`)**

Rejected. Pollutes the top-level resource namespace with account-level
concepts. No natural place for `analyze` or `bulk` operations.

**B. Flat verbs (`dtctl get users --scope account`)**

Rejected. Conflates two different APIs behind a flag. Makes it easy to
accidentally target the wrong scope. Does not scale to IAM-specific verbs
like `analyze` and `bulk`.

### Handling Existing `get users` / `get groups`

The existing `dtctl get users` and `dtctl get groups` commands use
environment-scoped IAM endpoints
(`/platform/iam/v1/organizational-levels/environment/{envID}/...`). These show
who has access to _this specific environment_.

**Decision:** Keep both. They serve different purposes:

| Command | Scope | Shows |
|---------|-------|-------|
| `dtctl get users` | Environment | Users with access to the current environment |
| `dtctl iam get users` | Account | All users in the Dynatrace account |
| `dtctl get groups` | Environment | Groups visible from the current environment |
| `dtctl iam get groups` | Account | All groups in the account, with full membership |

If `account-uuid` is not configured, `dtctl get users` output includes a hint:
`Tip: Use "dtctl iam get users" for the full account-level view`.

---

## Decision 2: Config Schema Extension

### Chosen: Add `account-uuid` and `account-token-ref` to Context

```yaml
# Current schema (unchanged fields)
contexts:
  - name: production
    context:
      environment: https://abc123.apps.dynatrace.com
      token-ref: prod-token
      safety-level: readwrite-all
      description: "Production environment"
      # NEW optional fields:
      account-uuid: 12345678-abcd-efgh-ijkl-123456789012
      account-token-ref: iam-cred          # optional, falls back to token-ref
```

```go
type Context struct {
    Environment     string      `yaml:"environment" table:"ENVIRONMENT"`
    TokenRef        string      `yaml:"token-ref" table:"TOKEN-REF"`
    SafetyLevel     SafetyLevel `yaml:"safety-level,omitempty" table:"SAFETY-LEVEL"`
    Description     string      `yaml:"description,omitempty" table:"DESCRIPTION,wide"`
    AccountUUID     string      `yaml:"account-uuid,omitempty" table:"ACCOUNT,wide"`
    AccountTokenRef string      `yaml:"account-token-ref,omitempty" table:"-"`
}
```

### Field Semantics

| Field | Required for | Default | Description |
|-------|-------------|---------|-------------|
| `account-uuid` | All `dtctl iam` commands | (none) | Dynatrace account UUID |
| `account-token-ref` | (optional) | Falls back to `token-ref` | Separate token/credential for account-level API. Enables using client-credentials for IAM while using PKCE for environment. |

### Is Account UUID Implicit from Tenant?

**No.** The relationship is one account to many environments. There is no
public API to look up which account owns an environment from the environment
side. Users must provide the account UUID explicitly.

### Discovery Workflow

```bash
# User knows their account UUID from Dynatrace Account Management UI
dtctl ctx set production --account-uuid 12345678-abcd-efgh-ijkl-123456789012

# Or: set account UUID on the current context
dtctl ctx set-account 12345678-abcd-efgh-ijkl-123456789012

# Verify it works
dtctl iam get environments
```

If `account-uuid` is missing, IAM commands fail with a clear message:

```
Error: account-uuid not configured for context "production"

Set it with:
  dtctl ctx set production --account-uuid YOUR_ACCOUNT_UUID

Find your account UUID in:
  Dynatrace Account Management > Account settings
```

### How Others Handle Account/Project Scoping

| CLI | Scoping concept | How it's set | Stored where |
|-----|----------------|--------------|-------------|
| **AWS CLI** | Account ID | Implicit from IAM credentials | Not in config (derived) |
| **Azure CLI** | Tenant + Subscription | `az account set --subscription ID` | `~/.azure/azureProfile.json` |
| **gcloud** | Project | `gcloud config set project ID` | `~/.config/gcloud/properties` |
| **kubectl** | Cluster | Embedded in context | `~/.kube/config` |
| **dtctl** (proposed) | Account UUID | `dtctl ctx set --account-uuid` | `~/.config/dtctl/config` |

The Azure approach is the closest analog: Dynatrace accounts are like Azure
tenants, and the scoping unit is a first-class config concept.

### Environment Variables

| Variable | Description |
|----------|-------------|
| `DTCTL_ACCOUNT_UUID` | Override account UUID (like dtiam's `DTIAM_ACCOUNT_UUID`) |
| `DTCTL_ACCOUNT_TOKEN` | Override account-level token |

These follow the existing `DTCTL_` prefix convention and take precedence over
config-file values.

---

## Decision 3: Authentication

### The Problem

dtctl currently supports:
- **PKCE (interactive):** Browser-based OAuth with the built-in client ID
  `dt0s12.dtctl-prod`. Scopes are requested per safety level.
- **API tokens:** Static tokens stored in keyring or config.

IAM needs:
- **Account-level OAuth scopes** (`account-idm-read`, `account-idm-write`,
  `iam-policies-management`, `account-env-read`).
- **Client-credentials flow** for automation/CI (no browser available).

### Chosen: Support Both Flows, Extend Scope Lists

#### Path A: Interactive Users (PKCE)

Extend the PKCE scope lists in `pkg/auth/oauth_flow.go` to include
account-level scopes when the context has `account-uuid` set:

| Safety Level | Added Scopes |
|-------------|-------------|
| `readonly` | `account-idm-read`, `account-env-read`, `iam:policies:read`, `iam:bindings:read`, `iam:boundaries:read`, `iam:effective-permissions:read` |
| `readwrite-mine` | Above + `account-idm-write` |
| `readwrite-all` | Above + `iam-policies-management` |
| `dangerously-unrestricted` | Same as `readwrite-all` |

**Caveat:** The built-in dtctl client ID (`dt0s12.dtctl-prod`) must be granted
these scopes by Dynatrace. If it is not, users get a clear OAuth error and are
directed to use their own credentials (Path B). This is a deployment concern,
not a code concern.

#### Path B: Automation / CI (Client Credentials)

Add client-credentials grant support to `pkg/auth/`:

```yaml
tokens:
  - name: iam-service-cred
    type: oauth-client-credentials     # NEW token type
    client-id: dt0s01.XXXXX
    # client-secret stored in keyring under "dtctl:iam-service-cred"

contexts:
  - name: production
    context:
      environment: https://abc123.apps.dynatrace.com
      token-ref: prod-token
      account-uuid: 12345678-abcd-...
      account-token-ref: iam-service-cred   # uses client-credentials
```

The client-credentials flow is implemented in `pkg/auth/` as a new
`ClientCredentialsProvider` that:
1. Takes `client-id` and `client-secret` (from keyring)
2. POSTs to `https://sso.dynatrace.com/sso/oauth2/token` with
   `grant_type=client_credentials`
3. Requests the account-level scopes
4. Caches and auto-refreshes the token

This is exactly what dtiam does today, adapted to dtctl's token storage.

#### Configuration Commands

```bash
# Store client credentials for IAM automation
dtctl ctx set-credentials iam-cred \
  --client-id dt0s01.XXXXX \
  --client-secret dt0s01.XXXXX.YYYYY

# Point the context's account-token-ref to it
dtctl ctx set production --account-token-ref iam-cred
```

#### Token Resolution Order for IAM Commands

1. `DTCTL_ACCOUNT_TOKEN` environment variable
2. `account-token-ref` in current context (may be client-credentials or regular)
3. `token-ref` in current context (PKCE token, if it has account scopes)
4. Error: "No valid token for account-level operations"

---

## Decision 4: HTTP Client Architecture

### The Problem

dtctl's `Client` struct has a single `baseURL` pointing to the environment.
IAM API calls go to `https://api.dynatrace.com`, a completely different host.

### Chosen: Account-Aware Client Extension

Rather than creating a completely separate client, extend the existing client
with account-level capabilities:

```go
// pkg/client/client.go

// AccountClient returns a client configured for the account management API.
// It uses the account-level token and api.dynatrace.com as base URL.
func (c *Client) AccountClient(accountUUID, token string) *AccountClient {
    return &AccountClient{
        http:        newRestyClient(accountBaseURL, token),
        accountUUID: accountUUID,
    }
}

// pkg/client/account_client.go

const accountBaseURL = "https://api.dynatrace.com"

// AccountClient handles requests to the Dynatrace Account Management API.
type AccountClient struct {
    http        *resty.Client
    accountUUID string
}

func (c *AccountClient) HTTP() *resty.Client { return c.http }
func (c *AccountClient) AccountUUID() string { return c.accountUUID }
```

IAM resource handlers receive an `*AccountClient` instead of the regular
`*Client`. This keeps the separation clean and prevents accidentally sending
environment-scoped tokens to the account API or vice versa.

### API Endpoints

All IAM endpoints are under `https://api.dynatrace.com/iam/v1/accounts/{accountUUID}/`:

| Resource | Path |
|----------|------|
| Users | `/users` |
| Groups | `/groups`, `/groups/{uuid}` |
| Service Users | `/service-users`, `/service-users/{uid}` |
| Policies | `/repo/{levelType}/{levelId}/policies` |
| Bindings | `/repo/{levelType}/{levelId}/bindings` |
| Boundaries | `/repo/{levelType}/{levelId}/boundaries` |
| Environments | `/environments` |
| Limits | `/limits` |

Policy levels: `account/{accountUUID}`, `environment/{envID}`, `global/global`.

### URL for Dev/Sprint Environments

The account management API base URL varies by environment tier:

| Tier | Base URL |
|------|----------|
| Production | `https://api.dynatrace.com` |
| Development | `https://api.dev.dynatracelabs.com` (TBD) |
| Sprint/Hardening | `https://api.sprint.dynatracelabs.com` (TBD) |

Auto-detection from the environment URL (same logic as `pkg/auth/` tier
detection) determines which account API base URL to use.

---

## Decision 5: Resource Handler Pattern

### Chosen: New Package `pkg/resources/iam/account/`

Place all account-level IAM handlers in a sub-package to distinguish from the
existing environment-scoped handlers in `pkg/resources/iam/`:

```
pkg/resources/iam/
    iam.go                    # Existing environment-scoped handlers (get users/groups)
    account/                  # NEW: account-level IAM handlers
        groups.go             # GroupHandler: List, Get, Create, Delete, Members
        users.go              # UserHandler: List, Get, Create, Delete, GroupMembership
        policies.go           # PolicyHandler: List, Get, Create, Delete (multi-level)
        bindings.go           # BindingHandler: List, Create, Delete (with boundaries)
        boundaries.go         # BoundaryHandler: List, Get, Create, Delete, Attach/Detach
        environments.go       # EnvironmentHandler: List, Get
        service_users.go      # ServiceUserHandler: CRUD + group management
        limits.go             # LimitsHandler: List, CheckCapacity
        permissions.go        # PermissionsAnalyzer: effective perms, matrix, least-privilege
```

Each handler follows dtctl's established pattern:

```go
type GroupHandler struct {
    client *client.AccountClient
}

type Group struct {
    UUID        string `json:"uuid" table:"UUID"`
    Name        string `json:"name" table:"NAME"`
    Description string `json:"description,omitempty" table:"DESCRIPTION,wide"`
    Owner       string `json:"owner,omitempty" table:"OWNER,wide"`
    MemberCount int    `json:"memberCount,omitempty" table:"MEMBERS"`
    CreatedAt   string `json:"createdAt,omitempty" table:"CREATED,wide"`
}

func (h *GroupHandler) List(filters map[string]string) ([]Group, error) { ... }
func (h *GroupHandler) Get(uuid string) (*Group, error) { ... }
func (h *GroupHandler) GetByName(name string) (*Group, error) { ... }
func (h *GroupHandler) Create(name, description string) (*Group, error) { ... }
func (h *GroupHandler) Delete(uuid string) error { ... }
func (h *GroupHandler) GetMembers(groupUUID string) ([]User, error) { ... }
func (h *GroupHandler) AddMember(groupUUID, email string) error { ... }
func (h *GroupHandler) RemoveMember(groupUUID, userUID string) error { ... }
```

### Output Integration

All structs use `table:""` tags for dtctl's output system. Golden tests are
added in `pkg/output/golden_test.go` using the real account-level structs.

---

## Decision 6: Safety Checks

All mutating `dtctl iam` commands require safety checks, following the
established pattern in `pkg/safety/`:

```go
// New operation types in pkg/safety/
const (
    OperationIAMCreate  Operation = "iam-create"   // groups, policies, bindings, boundaries, users
    OperationIAMUpdate  Operation = "iam-update"   // group membership, binding boundaries
    OperationIAMDelete  Operation = "iam-delete"   // any IAM resource deletion
)
```

### Permission Matrix for IAM Operations

| Safety Level | iam get/describe | iam create | iam delete | iam analyze |
|-------------|-----------------|------------|------------|-------------|
| `readonly` | yes | no | no | yes |
| `readwrite-mine` | yes | yes | own only (*) | yes |
| `readwrite-all` | yes | yes | yes | yes |
| `dangerously-unrestricted` | yes | yes | yes | yes |

(*) Ownership for IAM resources is determined by the `owner` field where
available. For resources without ownership (e.g., policy bindings),
`readwrite-mine` blocks deletion.

### Dry-Run Support

All mutating commands support `--dry-run`:

```bash
dtctl iam create group --name "Test" --dry-run
# Would create group "Test" in account 12345678-abcd-...
# (dry-run: no changes made)
```

Safety checks are skipped in dry-run mode (consistent with existing behavior).

---

## Decision 7: Keyring Token Storage

### Account-Level Token Keys

Account-level tokens use a distinct keyring key pattern to avoid collisions
with environment-scoped tokens:

| Token type | Keyring key pattern | Example |
|-----------|-------------------|---------|
| Environment (existing) | `oauth:{env}:{tokenRef}` | `oauth:prod:my-token` |
| Account (new) | `account:{accountUUID}:{tokenRef}` | `account:1234-abcd:iam-cred` |
| Client credentials (new) | `client-cred:{name}` | `client-cred:iam-service` |

### Client Secret Storage

For client-credentials tokens, the client secret is stored in keyring:

```bash
dtctl ctx set-credentials iam-cred --client-id dt0s01.XXX --client-secret dt0s01.XXX.YYY
# client-secret stored in keyring as "dtctl:client-cred:iam-cred"
# config file stores only: { name: iam-cred, type: oauth-client-credentials, client-id: dt0s01.XXX }
```

The client secret never appears in the YAML config file, consistent with
dtctl's existing keyring-first approach.

---

## Decision 8: Agent Mode

The `--agent` / `-A` JSON envelope works transparently for IAM commands because
IAM handlers return data through dtctl's existing output system:

```bash
dtctl iam get groups -A
```

```json
{
  "ok": true,
  "result": [
    {"uuid": "...", "name": "DevOps Team", "memberCount": 12}
  ],
  "context": {
    "verb": "iam-get",
    "resource": "groups",
    "account": "12345678-abcd-...",
    "suggestions": ["dtctl iam describe group 'DevOps Team'"]
  }
}
```

The `context.account` field is new and included for all IAM commands so agents
know which account the data came from.

---

## What Not to Port from dtiam

| dtiam feature | Reason to skip |
|--------------|---------------|
| Separate config file (`~/.config/dtiam/config`) | dtctl's config is more mature (keyring, safety levels, aliases) |
| Custom HTTP client | dtctl's `pkg/client/` has retry, rate limiting, pagination |
| Output formatting | dtctl's `pkg/output/` is richer (charts, agent envelope, color control, golden tests) |
| Global state singleton (`internal/cli/state.go`) | dtctl passes config through function parameters |
| Template engine | dtctl's `apply -f` with YAML/JSON already covers declarative creation |
| dtiam's env var names (`DTIAM_*`) | Use `DTCTL_ACCOUNT_UUID`, `DTCTL_ACCOUNT_TOKEN` instead |

### What to Port (Logic, Not Code)

- **Resource handler logic** -- API paths, request/response parsing, name
  resolution. Rewritten to use dtctl's `AccountClient` and struct patterns.
- **Permissions analysis** -- effective permissions calculator, matrix
  generation, least-privilege audit. Novel functionality not in dtctl today.
- **Bulk operations** -- CSV/YAML/JSON file processing for batch user/group
  management. Adapted to dtctl's `--dry-run` and safety check patterns.

---

## Implementation Phases

### Phase 1: Foundation

**Goal:** Config + auth + smoke test.

- Add `account-uuid` and `account-token-ref` to `Context` struct
- Add `dtctl ctx set-account` / `dtctl ctx set --account-uuid` commands
- Implement `AccountClient` in `pkg/client/`
- Add client-credentials grant flow to `pkg/auth/`
- Extend PKCE scope lists with account-level scopes
- Add `DTCTL_ACCOUNT_UUID` / `DTCTL_ACCOUNT_TOKEN` env var support
- Implement `dtctl iam whoami` as a connectivity smoke test
- Add account-level keyring key patterns

### Phase 2: Read-Only IAM

**Goal:** Full read access to all IAM resources.

- Implement handlers in `pkg/resources/iam/account/`:
  - `GroupHandler` (List, Get, GetByName, GetMembers)
  - `UserHandler` (List, Get, GetByEmail, GetGroups)
  - `PolicyHandler` (List, Get, multi-level: account/environment/global)
  - `BindingHandler` (List, GetForGroup)
  - `BoundaryHandler` (List, Get, GetAttachedPolicies)
  - `EnvironmentHandler` (List, Get)
  - `ServiceUserHandler` (List, Get, GetGroups)
  - `LimitsHandler` (List, CheckCapacity)
- Register `dtctl iam get <resource>` and `dtctl iam describe <resource>` commands
- Add golden tests for all resource types
- Add `--agent` context enrichment

### Phase 3: Mutating Operations

**Goal:** Create, update, delete for all IAM resources.

- `iam create group/policy/binding/boundary/service-user`
- `iam delete group/policy/binding/boundary/user/service-user`
- Group membership management (add-member, remove-member)
- User group management (add-to-groups, remove-from-groups)
- Safety checks with `OperationIAMCreate/Update/Delete`
- `--dry-run` for all mutations
- Confirmation prompts for destructive operations

### Phase 4: Advanced Features

**Goal:** Analysis and bulk operations.

- `iam analyze user-permissions` -- effective permissions calculator
- `iam analyze permissions-matrix` -- cross-reference groups/policies/users
- `iam analyze least-privilege` -- identify over-permissioned groups
- `iam bulk add-users-to-group -f users.csv`
- `iam bulk create-groups -f groups.yaml`
- `iam bulk create-bindings -f bindings.yaml`
- `iam export all` -- backup/migration export

---

## Open Questions

1. **Built-in client ID scopes.** Does `dt0s12.dtctl-prod` have (or can it
   get) the account-level OAuth scopes? If not, client-credentials is the only
   path for IAM, which hurts interactive UX.

2. **Account API base URL for non-prod tiers.** The production endpoint is
   `api.dynatrace.com`. What are the dev/sprint equivalents? Need to confirm
   with Dynatrace.

3. **`dtctl iam apply -f`?** Should IAM resources support the declarative
   `apply` pattern (idempotent create-or-update from YAML)? The prototype
   supports it. This would be phase 4+ and requires careful safety
   consideration for IAM resources.

4. **Rate limiting.** The account management API may have different rate limits
   than the environment API. Need to test and configure retry behavior on the
   `AccountClient` accordingly.

5. **Pagination.** The account IAM API uses different pagination parameters
   than the environment APIs. Need to identify the exact param names and
   whether page tokens embed filters (like Settings API) or require resending
   them (like Document API).

6. **Token platform support.** dtiam supports `get/create/delete tokens` for
   platform tokens. Should this be part of `dtctl iam` or a separate
   `dtctl token` command? Platform tokens are conceptually different from IAM
   but live on the same API plane.

---

## Appendix A: OAuth Scope Reference

### Account-Level Scopes (from dtiam documentation)

| Scope | Purpose |
|-------|---------|
| `account-idm-read` | List/get groups, users, service users, limits |
| `account-idm-write` | Create/delete groups, users, service users |
| `account-env-read` | List environments in account |
| `iam-policies-management` | Full policy, binding, and boundary CRUD |
| `iam:effective-permissions:read` | Effective permissions analysis |
| `iam:policies:read` | Read policies (subset of full management) |
| `iam:bindings:read` | Read bindings (subset of full management) |
| `iam:boundaries:read` | Read boundaries (subset of full management) |

### Read-Only Set

`account-idm-read`, `account-env-read`, `iam:policies:read`,
`iam:bindings:read`, `iam:boundaries:read`, `iam:effective-permissions:read`

### Full Management Set

`account-idm-read`, `account-idm-write`, `account-env-read`,
`iam-policies-management`, `iam:effective-permissions:read`

## Appendix B: dtiam Feature Matrix vs dtctl IAM (Proposed)

| dtiam Feature | dtctl iam Phase | Notes |
|--------------|----------------|-------|
| get/describe groups, users | Phase 2 | |
| get/describe policies, bindings | Phase 2 | |
| get/describe boundaries | Phase 2 | |
| get environments | Phase 2 | |
| get/create/delete service users | Phase 3 | |
| create/delete groups, users | Phase 3 | |
| create/delete policies, bindings | Phase 3 | |
| create/delete boundaries | Phase 3 | |
| group membership management | Phase 3 | |
| boundary attach/detach | Phase 3 | |
| account limits & subscriptions | Phase 2 | |
| analyze user-permissions | Phase 4 | |
| analyze permissions-matrix | Phase 4 | |
| analyze least-privilege | Phase 4 | |
| bulk operations | Phase 4 | |
| export all | Phase 4 | |
| template system | Not planned | dtctl `apply -f` covers this |
| `--plain` mode | Automatic | dtctl's `--plain` applies globally |
| `--dry-run` | Phase 3 | dtctl's `--dry-run` applies globally |
| Multi-context config | Phase 1 | Unified into dtctl's config |
| OAuth2 client credentials | Phase 1 | |
| Bearer token auth | Phase 1 | Via existing dtctl token support |
| Platform tokens CRUD | TBD | See open question 6 |
| App Engine registry | Not planned | Already exists as `dtctl get apps` (env-level) |
| Settings schema search | Not planned | Already exists as `dtctl get settings` (env-level) |

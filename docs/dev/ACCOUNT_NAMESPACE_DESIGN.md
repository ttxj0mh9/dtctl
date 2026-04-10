# Account Namespace Design

**Status:** Design Proposal
**Created:** 2026-04-10
**Author:** dtctl team
**Depends on:** [IAM_INTEGRATION_DESIGN.md](IAM_INTEGRATION_DESIGN.md) (config schema, AccountClient, auth)

## Overview

This document proposes a `dtctl account` subcommand namespace for non-IAM
resources on the Dynatrace Account Management API (`api.dynatrace.com`). While
the IAM integration design covers identity and access management, the account
management plane exposes additional API surfaces -- subscriptions, cost,
audit logs, notifications, and environment management -- that don't belong
under `dtctl iam` but share the same infrastructure (account UUID, account-level
auth, `api.dynatrace.com` base URL).

This proposal defines what goes into `dtctl account`, how it relates to
`dtctl iam`, and which APIs are worth supporting.

## Goals

1. **Single entry point** for all non-IAM account-level operations
2. **FinOps visibility** -- subscription cost, usage, and forecast from the CLI
3. **Audit trail** -- account-level audit logs accessible without the web UI
4. **Shared infrastructure** -- reuse config schema, `AccountClient`, and auth
   from the IAM integration (no new config fields needed)
5. **Clear boundary** between environment-plane and account-plane commands

## Non-Goals

- Replacing the Dynatrace Account Management web UI
- Supporting Dynatrace Managed cluster-level APIs
- Implementing cost allocation write operations in the initial phase
- Duplicating functionality that already exists at the environment level

---

## Background: Account Management API Surface

The Dynatrace Account Management API at `api.dynatrace.com` exposes these
distinct API domains, each with its own base path:

| Domain | Base Path | Auth Scope | Purpose |
|--------|-----------|------------|---------|
| **IAM** | `/iam/v1/accounts/{uuid}/...` | `account-idm-read/write` | Users, groups, policies, bindings, boundaries |
| **Subscriptions** | `/sub/v2/accounts/{uuid}/...` | `account-uac-read` | DPS subscriptions, usage, cost |
| **Cost allocation** | `/v1/subscriptions/{uuid}/cost-allocation` | `account-uac-read` | Cost breakdown by cost center / product |
| **Cost allocation mgmt** | `/v1/accounts/{uuid}/settings/...` | `account-uac-read/write` | Cost center and product CRUD |
| **Environments** | `/env/v1/accounts/{uuid}/environments` | `account-env-read` | List environments + management zones |
| **Audit logs** | `/audit/v1/accounts/{uuid}` | `account-idm-read` | Account-level change audit trail |
| **Notifications** | `/v1/accounts/{uuid}/notifications` | (TBD) | Budget, cost, forecast, BYOK alerts |
| **Limits** | `/iam/v1/accounts/{uuid}/limits` | `account-idm-read` | Account resource quotas |
| **Reference data** | `/ref/v1/...` | (TBD) | Time zones, geographic regions |

The IAM design covers the first row and the last two rows (limits and
reference data live under the `/iam/` path). This document covers everything
else.

### What Belongs Where

| Namespace | Resources | Rationale |
|-----------|-----------|-----------|
| `dtctl iam` | Users, groups, service users, policies, bindings, boundaries, limits | Identity and access control -- who can do what |
| `dtctl account` | Subscriptions, cost, usage, audit logs, notifications, environments, cost allocation | Account administration -- what you have, what it costs, what changed |
| Top-level (`dtctl get/describe`) | All existing environment-level resources | Environment plane -- stays unchanged |

The split follows domain boundaries, not API paths. Limits live under `/iam/`
in the API but are identity-related (max users, max groups). Environments live
under `/env/` but are needed by both namespaces -- `dtctl iam` for policy
scoping, `dtctl account` for the full environment list with management zones.

---

## Decision 1: Command Structure

### Chosen: `dtctl account <verb> <resource>` subcommand tree

```
# Subscriptions
dtctl account get subscriptions
dtctl account describe subscription <uuid>

# Usage & Cost
dtctl account get usage --subscription <uuid> [--env <id>] [--capability <key>]
dtctl account get cost --subscription <uuid> [--env <id>] [--capability <key>]
dtctl account get cost --subscription <uuid> --per-environment --from 2026-01-01 --to 2026-03-31
dtctl account get forecast

# Audit Logs
dtctl account get audit-logs [--from <time>] [--to <time>] [--filter <expr>]

# Notifications
dtctl account get notifications [--type BUDGET|COST|FORECAST|BYOK_REVOKED|BYOK_ACTIVATED]
                                [--severity SEVERE|WARN|INFO]
                                [--from <time>] [--to <time>]

# Environments (account-level view)
dtctl account get environments

# Cost Allocation
dtctl account get cost-centers
dtctl account get products
dtctl account get cost-allocation --subscription <uuid> --env <id> --field COSTCENTER|PRODUCT
```

### Grammar Deviation

Like `dtctl iam`, this is namespace-first rather than verb-first. The same
rationale applies (see [IAM_INTEGRATION_DESIGN.md, Decision 1](IAM_INTEGRATION_DESIGN.md#decision-1-command-structure)):

- Different API plane (account vs environment)
- Different auth requirements (`account-uac-read` scope)
- Different identity anchor (account UUID, not environment ID)
- Discoverability: `dtctl account --help` shows all account-level operations

### Relationship to `dtctl iam`

`dtctl account` and `dtctl iam` are **siblings**, not parent-child:

```
dtctl
  ├── get / describe / create / delete / ...   (environment plane)
  ├── iam                                       (account plane: identity & access)
  └── account                                   (account plane: administration)
```

Nesting `iam` under `account` (i.e., `dtctl account iam get users`) was
considered and rejected -- it creates four-level-deep commands for the most
common IAM operations and obscures the IAM namespace behind a generic parent.

Both namespaces share:
- The same `account-uuid` config field (from IAM design, Decision 2)
- The same `AccountClient` (from IAM design, Decision 4)
- The same account-level token resolution (from IAM design, Decision 3)

No new config fields are needed. If `account-uuid` is already configured for
`dtctl iam`, `dtctl account` commands work immediately.

### Why Not Top-Level?

Putting subscription/cost commands at the top level (`dtctl get subscriptions`)
would:

1. **Blur the API plane boundary.** Users wouldn't know that `get subscriptions`
   requires account-level auth while `get workflows` uses environment auth.
2. **Create naming collisions.** `dtctl get environments` could mean "list
   environment-level resources" or "list environments in my account."
3. **Complicate help text.** Every `get --help` would need to explain which
   resources are account-scoped vs environment-scoped.

The namespace makes the scope explicit. When you type `dtctl account`, you
know you're operating on the account management plane.

---

## Decision 2: Subscription & Cost Resources

### The DPS Subscription API

The Dynatrace Platform Subscription (DPS) API is the highest-value addition.
It provides subscription metadata, usage telemetry, cost data, and cost
forecasting -- all read-only.

### API Endpoints

| Endpoint | Method | Path | Description |
|----------|--------|------|-------------|
| List subscriptions | GET | `/sub/v2/accounts/{uuid}/subscriptions` | All subscriptions with summary info |
| Get subscription | GET | `/sub/v2/accounts/{uuid}/subscriptions/{subUuid}` | Full details: budget, periods, capabilities |
| Get usage | GET | `/sub/v2/accounts/{uuid}/subscriptions/{subUuid}/usage` | Aggregated usage by capability |
| Get usage/env | GET | `/sub/v2/accounts/{uuid}/subscriptions/{subUuid}/environments/usage` | Usage split by environment |
| Get cost | GET | `/sub/v2/accounts/{uuid}/subscriptions/{subUuid}/cost` | Aggregated cost by date |
| Get cost/env | GET | `/sub/v3/accounts/{uuid}/subscriptions/{subUuid}/environments/cost` | Cost split by environment (paginated) |
| Get forecast | GET | `/sub/v2/accounts/{uuid}/subscriptions/forecast` | Budget consumption forecast |
| Get events | GET | `/sub/v2/accounts/{uuid}/subscriptions/events` | Cost/forecast/budget events |

**Auth scope:** `account-uac-read` for all read operations.

### Data Structures

```go
// pkg/resources/account/subscription.go

type Subscription struct {
    UUID      string `json:"uuid" table:"UUID"`
    Type      string `json:"type" table:"TYPE"`
    SubType   string `json:"subType,omitempty" table:"SUBTYPE,wide"`
    Name      string `json:"name" table:"NAME"`
    Status    string `json:"status" table:"STATUS"`
    StartTime string `json:"startTime" table:"START"`
    EndTime   string `json:"endTime" table:"END"`
}

type SubscriptionDetail struct {
    Subscription
    Account       SubscriptionAccount       `json:"account" table:"-"`
    Budget        SubscriptionBudget        `json:"budget" table:"-"`
    CurrentPeriod SubscriptionCurrentPeriod `json:"currentPeriod" table:"-"`
    Periods       []SubscriptionPeriod      `json:"periods,omitempty" table:"-"`
    Capabilities  []SubscriptionCapability  `json:"capabilities,omitempty" table:"-"`
}

type SubscriptionBudget struct {
    Total        float64 `json:"total" table:"TOTAL"`
    Used         float64 `json:"used" table:"USED"`
    CurrencyCode string  `json:"currencyCode" table:"CURRENCY"`
}

type SubscriptionCurrentPeriod struct {
    StartTime     string `json:"startTime"`
    EndTime       string `json:"endTime"`
    DaysRemaining int    `json:"daysRemaining"`
}

type SubscriptionCapability struct {
    Key  string `json:"key" table:"KEY"`
    Name string `json:"name" table:"CAPABILITY"`
}
```

```go
// pkg/resources/account/usage.go

type Usage struct {
    CapabilityKey  string  `json:"capabilityKey" table:"CAPABILITY"`
    CapabilityName string  `json:"capabilityName" table:"NAME"`
    StartTime      string  `json:"startTime" table:"START"`
    EndTime        string  `json:"endTime" table:"END"`
    Value          float64 `json:"value" table:"VALUE"`
    UnitMeasure    string  `json:"unitMeasure" table:"UNIT"`
}

type EnvironmentUsage struct {
    EnvironmentID string  `json:"environmentId" table:"ENVIRONMENT"`
    Usage         []Usage `json:"usage" table:"-"`
}
```

```go
// pkg/resources/account/cost.go

type Cost struct {
    StartTime       string  `json:"startTime" table:"START"`
    EndTime         string  `json:"endTime" table:"END"`
    Value           float64 `json:"value" table:"COST"`
    CurrencyCode    string  `json:"currencyCode" table:"CURRENCY"`
    LastBookingDate string  `json:"lastBookingDate,omitempty" table:"BOOKED,wide"`
}

type EnvironmentCost struct {
    EnvironmentID string           `json:"environmentId" table:"ENVIRONMENT"`
    ClusterID     string           `json:"clusterId,omitempty" table:"-"`
    Cost          []CapabilityCost `json:"cost" table:"-"`
}

type CapabilityCost struct {
    StartTime      string  `json:"startTime" table:"START"`
    EndTime        string  `json:"endTime" table:"END"`
    Value          float64 `json:"value" table:"COST"`
    CurrencyCode   string  `json:"currencyCode" table:"CURRENCY"`
    CapabilityKey  string  `json:"capabilityKey" table:"CAPABILITY"`
    CapabilityName string  `json:"capabilityName" table:"NAME"`
    BookingDate    string  `json:"bookingDate,omitempty" table:"BOOKED,wide"`
}

type Forecast struct {
    ForecastMedian    float64 `json:"forecastMedian" table:"FORECAST (MEDIAN)"`
    ForecastLower     float64 `json:"forecastLower" table:"LOWER"`
    ForecastUpper     float64 `json:"forecastUpper" table:"UPPER"`
    Budget            float64 `json:"budget" table:"BUDGET"`
    ForecastBudgetPct float64 `json:"forecastBudgetPct" table:"BUDGET %"`
    ForecastBudgetDate string `json:"forecastBudgetDate,omitempty" table:"EXHAUSTION DATE"`
    ForecastCreatedAt  string `json:"forecastCreatedAt,omitempty" table:"CREATED,wide"`
}
```

### Command UX Examples

**List subscriptions:**

```bash
$ dtctl account get subscriptions
UUID                                  NAME                 TYPE   STATUS   START        END
a1b2c3d4-e5f6-7890-abcd-ef1234567890  Enterprise DPS 2026  DPS    ACTIVE   2026-01-01   2026-12-31
```

**Describe subscription (detailed view):**

```bash
$ dtctl account describe subscription a1b2c3d4
Name:           Enterprise DPS 2026
UUID:           a1b2c3d4-e5f6-7890-abcd-ef1234567890
Type:           DPS
Status:         ACTIVE
Start:          2026-01-01
End:            2026-12-31

Budget:
  Total:        500000.00 EUR
  Used:         127340.50 EUR (25.5%)
  Remaining:    372659.50 EUR

Current Period:
  Start:        2026-01-01
  End:          2026-12-31
  Days Left:    265

Capabilities:
  KEY                         NAME
  LOG_MANAGEMENT_ANALYZE      Log Management & Analytics - Query
  CUSTOM_METRICS              Custom Metrics
  SYNTHETIC_ACTIONS           Synthetic Monitoring - Actions
  ...
```

**View cost per environment for current quarter:**

```bash
$ dtctl account get cost --subscription a1b2c3d4 --per-environment \
    --from 2026-01-01 --to 2026-03-31
ENVIRONMENT   CAPABILITY                  COST       CURRENCY
abc123        Log Management - Query      12450.00   EUR
abc123        Custom Metrics               3200.00   EUR
def456        Log Management - Query       8900.00   EUR
def456        Synthetic - Actions          1550.00   EUR
```

**View forecast:**

```bash
$ dtctl account get forecast
FORECAST (MEDIAN)  LOWER      UPPER      BUDGET     BUDGET %   EXHAUSTION DATE
  487200.00        461000.00  513400.00  500000.00  97.4%      (none)
```

### Subscription Auto-Selection

Many accounts have a single active subscription. When `--subscription` is
required but not provided:

1. List subscriptions, filter to `status == "ACTIVE"`.
2. If exactly one active subscription exists, use it automatically.
3. If multiple exist, prompt interactively (or fail in `--plain` mode).
4. Show which subscription was auto-selected in stderr:
   `Using subscription "Enterprise DPS 2026" (a1b2c3d4-...)`

This follows dtctl's existing interactive name resolution pattern.

---

## Decision 3: Audit Logs

### API Endpoint

| Method | Path | Auth Scope |
|--------|------|------------|
| GET | `/audit/v1/accounts/{uuid}` | `account-idm-read` |

Supports filtering by time range, resource type, resource name, and boolean
expressions. Returns detailed records including who, what, when, where,
before/after JSON diffs, and success/failure status.

### Command Design

```bash
# Recent audit logs (default: last 24h)
dtctl account get audit-logs

# Time-filtered
dtctl account get audit-logs --from 2026-04-01 --to 2026-04-10

# Expression filter (passed through to API)
dtctl account get audit-logs --filter "resource = 'POLICY' and eventType = 'DELETE'"
dtctl account get audit-logs --filter "resourceName contains 'admin'"

# With full details
dtctl account get audit-logs -o wide
```

### Data Structure

```go
// pkg/resources/account/audit.go

type AuditLog struct {
    EventID       string `json:"eventId" table:"EVENT-ID"`
    Timestamp     string `json:"timestamp" table:"TIMESTAMP"`
    User          string `json:"user" table:"USER"`
    Resource      string `json:"resource" table:"RESOURCE"`
    ResourceName  string `json:"resourceName" table:"NAME"`
    EventType     string `json:"eventType" table:"TYPE"`
    EventOutcome  string `json:"eventOutcome" table:"OUTCOME"`
    // Wide fields
    ResourceID    string `json:"resourceId,omitempty" table:"RESOURCE-ID,wide"`
    EventProvider string `json:"eventProvider,omitempty" table:"PROVIDER,wide"`
    OriginAddress string `json:"originAddress,omitempty" table:"ORIGIN,wide"`
    TenantID      string `json:"tenantId,omitempty" table:"TENANT,wide"`
    // Detail-only fields (for describe)
    AuthType      string                 `json:"authenticationType,omitempty" table:"-"`
    AuthGrantType string                 `json:"authenticationGrantType,omitempty" table:"-"`
    Details       map[string]interface{} `json:"details,omitempty" table:"-"`
    EventReason   string                 `json:"eventReason,omitempty" table:"-"`
}
```

### Table Output

```bash
$ dtctl account get audit-logs --from 2026-04-09
EVENT-ID                              TIMESTAMP                  USER                 RESOURCE  NAME           TYPE    OUTCOME
af1f98c9-c611-4056-841b-d039b1af3f98  2026-04-09T15:25:41.893Z   user@example.invalid  POLICY    Standard User  CREATE  SUCCESS
bf2fa7d8-d722-5167-952c-e14ac2bg4g09  2026-04-09T14:10:22.100Z   user@example.invalid  GROUP     DevOps Team    UPDATE  SUCCESS
```

### Describe (Single Audit Entry)

Audit logs don't have a natural "describe" pattern since they're event
records, not resources. Instead, use `-o yaml` or `-o json` for full detail
including before/after diffs:

```bash
dtctl account get audit-logs --filter "eventId = 'af1f98c9-...'" -o yaml
```

---

## Decision 4: Notifications

### API Endpoint

| Method | Path | Auth Scope |
|--------|------|------------|
| POST | `/v1/accounts/{uuid}/notifications` | TBD (likely `account-uac-read`) |

This is a POST-with-filter-body endpoint, which is unusual. The filter is
submitted as a JSON request body, not query parameters.

### Command Design

```bash
# All notifications
dtctl account get notifications

# Filtered by type and severity
dtctl account get notifications --type BUDGET --severity SEVERE
dtctl account get notifications --type FORECAST,COST --from 2026-01-01

# BYOK notifications (security-relevant)
dtctl account get notifications --type BYOK_REVOKED,BYOK_ACTIVATED
```

### Data Structure

```go
// pkg/resources/account/notification.go

type Notification struct {
    Key         string `json:"key" table:"KEY"`
    Message     string `json:"message" table:"MESSAGE"`
    Severity    string `json:"severity" table:"SEVERITY"`
    Type        string `json:"type" table:"TYPE"`
    Date        string `json:"date" table:"DATE"`
    AccountUUID string `json:"accountUuid" table:"-"`
}
```

### Implementation Note: POST as GET

The notifications API uses POST for what is semantically a read operation
(listing notifications with filters). The handler should:

1. Accept the same CLI flags as a GET command would.
2. Translate flags into the POST request body.
3. Present results identically to other `get` commands.

The user never sees that this is a POST underneath. This is consistent with
how dtctl handles DQL queries (POST with query body, presented as `dtctl query`).

---

## Decision 5: Environment Management

### API Endpoint

| Method | Path | Auth Scope |
|--------|------|------------|
| GET | `/env/v1/accounts/{uuid}/environments` | `account-env-read` |

Returns all environments and their management zones.

### Overlap with `dtctl iam get environments`

The IAM design includes `dtctl iam get environments` for the same endpoint.
Both use `/env/v1/accounts/{uuid}/environments`. Rather than having two
commands hit the same API, define clear ownership:

| Command | Purpose | Includes MZs |
|---------|---------|--------------|
| `dtctl iam get environments` | Quick list for policy scoping context | No (just IDs and names) |
| `dtctl account get environments` | Full environment inventory with management zones | Yes |

Both use the same handler under the hood. The `iam` variant is a convenience
alias that omits management zone details. The `account` variant is the
canonical command for environment discovery.

### Data Structure

```go
// pkg/resources/account/environment.go

type Environment struct {
    ID   string `json:"id" table:"ID"`
    Name string `json:"name" table:"NAME"`
}

type ManagementZone struct {
    ID     string `json:"id" table:"ID"`
    Name   string `json:"name" table:"NAME"`
    Parent string `json:"parent" table:"ENVIRONMENT"`
}
```

### Table Output

```bash
$ dtctl account get environments
ID        NAME
abc123    Production
def456    Staging
ghi789    Development

$ dtctl account get environments -o wide
# Includes management zones in a sub-table or nested output
```

### Cross-Reference with Contexts

This command is especially useful for context setup:

```bash
# Discover which environments exist in the account
dtctl account get environments

# Then create contexts for them
dtctl ctx set production --environment https://abc123.apps.dynatrace.com
dtctl ctx set staging --environment https://def456.apps.dynatrace.com
```

A future enhancement could offer `dtctl account setup-contexts` to
auto-create contexts for all environments in the account, but that is out
of scope for this proposal.

---

## Decision 6: Cost Allocation

### API Endpoints

**Read (Phase 1):**

| Method | Path | Auth Scope |
|--------|------|------------|
| GET | `/v1/accounts/{uuid}/settings/costcenters` | `account-uac-read` |
| GET | `/v1/accounts/{uuid}/settings/products` | `account-uac-read` |
| GET | `/v1/subscriptions/{uuid}/cost-allocation` | `account-uac-read` |

**Write (Phase 2+):**

| Method | Path | Auth Scope |
|--------|------|------------|
| POST | `/v1/accounts/{uuid}/settings/costcenters` | `account-uac-write` |
| PUT | `/v1/accounts/{uuid}/settings/costcenters` | `account-uac-write` |
| DELETE | `/v1/accounts/{uuid}/settings/costcenters/{key}` | `account-uac-write` |
| POST | `/v1/accounts/{uuid}/settings/products` | `account-uac-write` |
| PUT | `/v1/accounts/{uuid}/settings/products` | `account-uac-write` |
| DELETE | `/v1/accounts/{uuid}/settings/products/{key}` | `account-uac-write` |

### Command Design

```bash
# List defined cost centers and products
dtctl account get cost-centers
dtctl account get products

# View cost allocation breakdown
dtctl account get cost-allocation --subscription <uuid> --env <id> --field COSTCENTER
dtctl account get cost-allocation --subscription <uuid> --env <id> --field PRODUCT

# (Phase 2+) Manage cost centers
dtctl account create cost-center --name "Engineering"
dtctl account delete cost-center "Engineering"
dtctl account create product --name "Platform Services"
dtctl account delete product "Platform Services"
```

### Rationale for Deferred Write Operations

Cost allocation write operations (`account-uac-write` scope) are deferred
to Phase 2+ because:

1. Read-only operations serve the primary use case (visibility into cost
   breakdown).
2. Write operations require safety checks and a new `account-uac-write` scope
   that may not be available through PKCE.
3. Cost center/product definitions are infrequently changed and are
   manageable through the web UI.

---

## Decision 7: Safety Checks

### Read-Only Commands

All Phase 1 `dtctl account` commands are **read-only** and require no safety
checks. This is a significant simplification compared to `dtctl iam`, which
has create/delete operations from Phase 3 onward.

| Command | Mutating | Safety Check |
|---------|----------|-------------|
| `account get subscriptions` | No | None |
| `account describe subscription` | No | None |
| `account get usage` | No | None |
| `account get cost` | No | None |
| `account get forecast` | No | None |
| `account get audit-logs` | No | None |
| `account get notifications` | No | None |
| `account get environments` | No | None |
| `account get cost-centers` | No | None |
| `account get products` | No | None |
| `account get cost-allocation` | No | None |

### Future Write Operations

If cost allocation writes are added (Phase 2+), they follow the standard
safety check pattern:

```go
const (
    OperationAccountCreate Operation = "account-create"
    OperationAccountUpdate Operation = "account-update"
    OperationAccountDelete Operation = "account-delete"
)
```

The safety matrix would match the IAM pattern: blocked at `readonly` and
`readwrite-mine`, allowed at `readwrite-all` and above.

---

## Decision 8: OAuth Scope Extension

### New Scope: `account-uac-read`

The IAM design adds `account-idm-read` and `account-env-read` to the PKCE
scope lists. The account namespace additionally needs `account-uac-read` for
subscription and cost data.

| Safety Level | Scopes Added by IAM Design | Additional for Account |
|-------------|---------------------------|----------------------|
| `readonly` | `account-idm-read`, `account-env-read`, `iam:policies:read`, ... | `account-uac-read` |
| `readwrite-mine` | Above + `account-idm-write` | `account-uac-read` |
| `readwrite-all` | Above + `iam-policies-management` | `account-uac-read`, `account-uac-write` |

`account-uac-read` is added at all safety levels because cost/usage data is
inherently read-only observability data -- there's no reason to gate it behind
a higher safety level.

`account-uac-write` (for cost allocation management) is added at
`readwrite-all` only, consistent with how IAM write scopes are handled.

### Token Resolution

The same token resolution order from the IAM design applies. No changes
needed:

1. `DTCTL_ACCOUNT_TOKEN` environment variable
2. `account-token-ref` in current context
3. `token-ref` in current context (if it has account scopes)
4. Error

---

## Decision 9: Agent Mode

The `--agent` / `-A` JSON envelope works transparently for account commands:

```bash
dtctl account get subscriptions -A
```

```json
{
  "ok": true,
  "result": [
    {"uuid": "a1b2c3d4-...", "name": "Enterprise DPS 2026", "status": "ACTIVE", ...}
  ],
  "context": {
    "verb": "account-get",
    "resource": "subscriptions",
    "account": "12345678-abcd-...",
    "suggestions": ["dtctl account describe subscription a1b2c3d4-..."]
  }
}
```

Cost and forecast data is especially useful for AI agents building FinOps
reports or automated cost monitoring.

---

## Decision 10: Help Text and Discoverability

### `dtctl --help` Layout

The `account` subcommand appears alongside `iam` under "Platform
Administration":

```
Platform Administration:
  iam         Manage Identity and Access Management (account-level)
  account     View subscriptions, cost, usage, and audit logs (account-level)
```

### `dtctl account --help` Layout

```
View and manage Dynatrace account-level resources: subscriptions, cost,
usage, audit logs, and notifications.

Operates at the account level (not environment level). Requires account-uuid
to be configured: dtctl ctx set --account-uuid UUID

Usage:
  dtctl account [command]

Resource Commands:
  get         List account resources (subscriptions, cost, audit-logs, ...)
  describe    Show detailed account resource information

Use "dtctl account [command] --help" for more information about a command.
```

---

## Decision 11: Pagination

### Cost Per Environment Endpoint

The `/sub/v3/.../environments/cost` endpoint is paginated with `page-key` and
`page-size`. The exact behavior (whether page-key embeds filters) needs to be
tested, but the safe default is the standard dtctl pagination pattern:

```go
for {
    req := h.client.HTTP().R().SetResult(&result)

    if nextPageKey != "" {
        req.SetQueryParam("page-key", nextPageKey)
    } else if chunkSize > 0 {
        req.SetQueryParam("page-size", fmt.Sprintf("%d", chunkSize))
    }
    // Always resend filters
    if startTime != "" {
        req.SetQueryParam("startTime", startTime)
    }
    if endTime != "" {
        req.SetQueryParam("endTime", endTime)
    }

    resp, err := req.Get(path)
    // ... handle response, break if no more pages
}
```

### Cost Allocation Endpoint

The `/v1/subscriptions/{uuid}/cost-allocation` endpoint uses `page-key` and
explicitly states "if this query parameter is set, no other query parameters
can be set." This is the Settings API pattern -- the page token embeds
everything:

```go
for {
    req := h.client.HTTP().R().SetResult(&result)

    if nextPageKey != "" {
        req.SetQueryParam("page-key", nextPageKey)
    } else {
        if field != "" {
            req.SetQueryParam("field", field)
        }
        if envID != "" {
            req.SetQueryParam("environment-id", envID)
        }
        if pageSize > 0 {
            req.SetQueryParam("page-size", fmt.Sprintf("%d", pageSize))
        }
    }

    resp, err := req.Get(path)
    // ... handle response, break if no more pages
}
```

### Cost Centers / Products

These use `page` and `page-size` (offset-based pagination, not cursor-based).
This is a different pattern from most dtctl resources:

```go
for page := 1; ; page++ {
    req := h.client.HTTP().R().SetResult(&result)
    req.SetQueryParam("page", fmt.Sprintf("%d", page))
    if pageSize > 0 {
        req.SetQueryParam("page-size", fmt.Sprintf("%d", pageSize))
    }

    resp, err := req.Get(path)
    // ... break if !result.HasNextPage
}
```

---

## Decision 12: Package Layout

```
pkg/resources/account/
    subscription.go      # SubscriptionHandler: List, Get
    usage.go             # UsageHandler: GetUsage, GetUsageByEnvironment
    cost.go              # CostHandler: GetCost, GetCostByEnvironment
    forecast.go          # ForecastHandler: GetForecast, GetEvents
    audit.go             # AuditHandler: List (with filters)
    notification.go      # NotificationHandler: List (with filters)
    environment.go       # EnvironmentHandler: List (shared with IAM)
    cost_allocation.go   # CostAllocationHandler: GetAllocation, GetCostCenters, GetProducts
```

Each handler takes an `*client.AccountClient` and follows dtctl's established
handler pattern. The `EnvironmentHandler` is shared between `dtctl account`
and `dtctl iam` -- both command trees call the same handler, with `iam`
presenting a simplified view.

---

## Decision 13: Error UX

Account namespace commands share the same error surface as IAM commands for
config and auth errors (no account-uuid, missing scopes, wrong account UUID).
The error messages from IAM design Decision 9 apply directly.

Additional account-specific errors:

### No Active Subscription

```
Error: no active subscriptions found in account "12345678-abcd-..."

This account has 0 active DPS subscriptions. If you recently activated
a subscription, it may take a few minutes to appear.

Check your subscription status in:
  Dynatrace Account Management > Subscription > Overview
```

### Subscription UUID Required

```
Error: --subscription flag is required

This account has multiple active subscriptions:
  a1b2c3d4  Enterprise DPS 2026  (active)
  e5f6a7b8  Platform Trial       (active)

Specify one with:
  dtctl account get cost --subscription a1b2c3d4
```

---

## Implementation Phases

### Phase 1: Subscriptions & Cost (Read-Only)

**Goal:** Core FinOps visibility from the CLI.

**Depends on:** IAM Phase 1 (config schema, `AccountClient`, auth)

- Implement `SubscriptionHandler` (List, Get)
- Implement `UsageHandler` (aggregated, per-environment)
- Implement `CostHandler` (aggregated, per-environment with pagination)
- Implement `ForecastHandler` (forecast, events)
- Register `dtctl account get subscriptions`, `describe subscription`
- Register `dtctl account get usage`, `get cost`, `get forecast`
- Subscription auto-selection for single-subscription accounts
- Add `account-uac-read` to PKCE scope lists
- Golden tests for all resource types
- Agent mode context enrichment

### Phase 2: Audit Logs, Notifications, Environments

**Goal:** Account administration visibility.

- Implement `AuditHandler` (list with time range and expression filters)
- Implement `NotificationHandler` (list with type/severity filters)
- Implement `EnvironmentHandler` (list with management zones)
- Register `dtctl account get audit-logs`, `get notifications`,
  `get environments`
- Wire environment handler to `dtctl iam get environments` as a shared
  implementation

### Phase 3: Cost Allocation

**Goal:** FinOps cost breakdown and management.

- Implement `CostAllocationHandler` (read allocation by field)
- Implement cost center / product read handlers
- Register `dtctl account get cost-allocation`, `get cost-centers`,
  `get products`
- (Optional) Cost center / product write operations with safety checks
  and `account-uac-write` scope

---

## Open Questions

1. **`account-uac-read` via PKCE.** Can the built-in dtctl client ID
   (`dt0s12.dtctl-prod`) be granted `account-uac-read`? If not,
   subscription/cost commands are client-credentials-only, which limits
   interactive use. This is the same open question as IAM's scope concern,
   but for a different scope.

2. **Notifications API scope.** The documentation doesn't clearly state which
   OAuth scope is needed for the notifications endpoint. Needs testing.

3. **Cost/environment pagination.** The `/sub/v3/.../environments/cost`
   endpoint supports `page-key` and `page-size`. Does it reject them together
   (standard pattern) or accept them together (Document API pattern)?
   Needs testing.

4. **Timeframe defaults.** When `--from`/`--to` are omitted for audit logs
   and notifications, what's the sensible default? Options:
   - Last 24 hours (matches monitoring conventions)
   - Last 7 days (matches audit review patterns)
   - Current subscription period (for cost/usage)

5. **Chart output for cost data.** dtctl supports `--chart` for some resource
   types. Cost-over-time and usage-over-time are natural candidates for
   sparkline or bar chart visualization. Worth including in Phase 1 or
   deferring?

---

## Appendix A: OAuth Scope Reference

### Scopes Required by Account Namespace

| Scope | Purpose | Phase |
|-------|---------|-------|
| `account-uac-read` | Subscriptions, usage, cost, cost allocation, cost centers, products | Phase 1 |
| `account-env-read` | Environment list (shared with IAM) | Phase 2 |
| `account-idm-read` | Audit logs (shared with IAM) | Phase 2 |
| `account-uac-write` | Cost center/product management | Phase 3 (optional) |

### Combined Scope Set (IAM + Account)

For a context with both IAM and account capabilities at `readonly`:

```
account-idm-read, account-env-read, account-uac-read,
iam:policies:read, iam:bindings:read, iam:boundaries:read,
iam:effective-permissions:read
```

## Appendix B: Full API Endpoint Reference

| Resource | Method | Path | Paginated | Phase |
|----------|--------|------|-----------|-------|
| List subscriptions | GET | `/sub/v2/accounts/{uuid}/subscriptions` | No | 1 |
| Get subscription | GET | `/sub/v2/accounts/{uuid}/subscriptions/{subUuid}` | No | 1 |
| Get usage | GET | `/sub/v2/accounts/{uuid}/subscriptions/{subUuid}/usage` | No | 1 |
| Get usage/env | GET | `/sub/v2/accounts/{uuid}/subscriptions/{subUuid}/environments/usage` | No | 1 |
| Get cost | GET | `/sub/v2/accounts/{uuid}/subscriptions/{subUuid}/cost` | No | 1 |
| Get cost/env | GET | `/sub/v3/accounts/{uuid}/subscriptions/{subUuid}/environments/cost` | Yes (cursor) | 1 |
| Get forecast | GET | `/sub/v2/accounts/{uuid}/subscriptions/forecast` | No | 1 |
| Get events | GET | `/sub/v2/accounts/{uuid}/subscriptions/events` | No | 1 |
| List audit logs | GET | `/audit/v1/accounts/{uuid}` | No (limit param) | 2 |
| List notifications | POST | `/v1/accounts/{uuid}/notifications` | Yes (page/pageSize) | 2 |
| List environments | GET | `/env/v1/accounts/{uuid}/environments` | No | 2 |
| Get cost allocation | GET | `/v1/subscriptions/{uuid}/cost-allocation` | Yes (cursor) | 3 |
| List cost centers | GET | `/v1/accounts/{uuid}/settings/costcenters` | Yes (offset) | 3 |
| List products | GET | `/v1/accounts/{uuid}/settings/products` | Yes (offset) | 3 |
| Create cost centers | POST | `/v1/accounts/{uuid}/settings/costcenters` | No | 3+ |
| Replace cost centers | PUT | `/v1/accounts/{uuid}/settings/costcenters` | No | 3+ |
| Delete cost center | DELETE | `/v1/accounts/{uuid}/settings/costcenters/{key}` | No | 3+ |
| Create products | POST | `/v1/accounts/{uuid}/settings/products` | No | 3+ |
| Replace products | PUT | `/v1/accounts/{uuid}/settings/products` | No | 3+ |
| Delete product | DELETE | `/v1/accounts/{uuid}/settings/products/{key}` | No | 3+ |

## Appendix C: What's Excluded (and Why)

| API / Feature | Reason to Exclude |
|--------------|------------------|
| Reference Data API (time zones, regions) | Pure reference data with no actionable CLI use case |
| Platform Tokens CRUD | Credentials management, not account administration. Under consideration as a separate top-level resource (`dtctl get tokens`) |
| Environment edit (name/timezone) | Low-value, infrequent operation best done in the web UI. Could be added later as `dtctl account edit environment` |
| Lens (Adoption & Environments dashboards) | Web-UI-only feature, no public API |
| SAML/SCIM configuration | IAM-adjacent but highly sensitive; web UI is the right interface |
| Contact/billing information | Account metadata management; web UI only |

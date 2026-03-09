# Send Event Design

**Status:** Design Proposal
**Created:** 2026-03-09
**Issue:** [#44](https://github.com/dynatrace-oss/dtctl/issues/44)

## Overview

The `send` verb adds data ingestion to dtctl. The first subcommand, `send event`, posts arbitrary event payloads to Dynatrace ingest endpoints. Users provide the JSON body; dtctl handles auth, transport, and error reporting.

This keeps dtctl's role simple: authenticate, send, report status. All event shaping, field selection, and payload construction are the user's responsibility — dtctl is a transport, not a schema enforcer.

## Goals

1. **Send user-supplied JSON** to Dynatrace platform ingest endpoints
2. **Support custom OpenPipeline endpoints** (`/platform/ingest/custom/events/<name>`)
3. **Support the built-in platform events endpoint** (`/platform/ingest/v1/events`)
4. **Batch support** — send multiple events from JSONL input
5. **Dry-run** — validate input is parseable JSON without sending
6. **Agent mode** — structured envelope output (`--agent` / `-A`)

All endpoints use **platform token scopes** (`domain:resource:permission` format). No classic API token scopes or classic environment API v2 endpoints.

## Non-Goals

- Schema validation against Dynatrace event schemas (user owns their payload)
- Event templating or payload construction beyond `--set` convenience
- Persistent event queuing or retry-on-failure beyond the standard HTTP client retry
- OpenPipeline configuration management (already covered by `dtctl create settings`)
- Classic environment API v2 endpoints (`/api/v2/...`) or classic token scopes

---

## User Experience

### Send from file

```bash
# Send to a custom OpenPipeline endpoint
dtctl send event -f activity.json --endpoint agent-activity

# Send to the built-in platform events endpoint (default when no --endpoint)
dtctl send event -f event.json
```

### Send from stdin

```bash
# Pipe from another command
echo '{"event.type":"custom","title":"deploy complete"}' | dtctl send event -f -

# Here-doc
dtctl send event -f - <<EOF
{"event.type": "custom", "title": "build finished", "build.id": "42"}
EOF
```

### Inline construction with --set

```bash
# Build a simple event inline (no file needed)
dtctl send event --set event.type=custom --set title="deploy done" --set build.id=42
```

`--set` builds a flat JSON object from key=value pairs. For anything non-trivial, use `-f`.

### Batch send (JSONL)

```bash
# Send multiple events, one JSON object per line
dtctl send event -f events.jsonl --batch

# Batch from stdin
cat events.jsonl | dtctl send event -f - --batch
```

### Dry run

```bash
# Validate JSON parses correctly, print what would be sent, don't send
dtctl send event -f event.json --dry-run
dtctl send event -f events.jsonl --batch --dry-run
```

### Custom endpoint categories

OpenPipeline custom endpoints exist under three URL prefixes. The `--category` flag selects which:

```bash
# Custom events (default)
# → POST /platform/ingest/custom/events/<name>
dtctl send event -f e.json --endpoint my-endpoint

# SDLC events
# → POST /platform/ingest/custom/events.sdlc/<name>
dtctl send event -f e.json --endpoint my-endpoint --category sdlc

# Security events
# → POST /platform/ingest/custom/security.events/<name>
dtctl send event -f e.json --endpoint my-endpoint --category security
```

### Output

Default (success):
```
Event sent (1 event, 234 bytes) → agent-activity
```

Batch:
```
Events sent (47 events, 12.3 KB) → agent-activity
```

Dry run:
```
Dry run: would send 1 event (234 bytes) → agent-activity
```

Agent mode (`-A`):
```json
{
  "ok": true,
  "result": {
    "events_sent": 1,
    "bytes": 234,
    "endpoint": "agent-activity"
  },
  "context": {
    "verb": "send",
    "resource": "event"
  }
}
```

---

## Endpoint Resolution

The `--endpoint` flag determines the target URL. All URLs are under the `/platform/` path — no classic environment API v2 endpoints are used.

| `--endpoint` value | `--category` | Target URL |
|---|---|---|
| *(omitted)* | — | `POST /platform/ingest/v1/events` |
| `<name>` | *(omitted / events)* | `POST /platform/ingest/custom/events/<name>` |
| `<name>` | `sdlc` | `POST /platform/ingest/custom/events.sdlc/<name>` |
| `<name>` | `security` | `POST /platform/ingest/custom/security.events/<name>` |

When `--endpoint` is provided, the value is always treated as a custom OpenPipeline endpoint name. There are no reserved names.

---

## Technical Design

### Command Structure

```
send (parent — requires subcommand)
└── event
```

The `send` parent command follows the `exec` pattern: it requires a subcommand and prints help if invoked alone. This leaves room for future subcommands (e.g., `send metric`, `send log`) without redesign.

### Files

| File | Purpose |
|---|---|
| `cmd/send.go` | Parent `send` command (~30 lines) |
| `cmd/send_event.go` | `send event` subcommand, flag registration, RunE |
| `pkg/resources/event/send.go` | HTTP transport: build URL, POST payload, parse response |
| `pkg/resources/event/send_test.go` | Unit tests |
| `test/e2e/send_event_test.go` | E2E tests |

### `cmd/send.go`

```go
var sendCmd = &cobra.Command{
    Use:   "send",
    Short: "Send data to Dynatrace platform ingest endpoints",
    Long:  "Send events to Dynatrace platform ingest endpoints.",
    RunE:  requireSubcommand,
}

func init() {
    rootCmd.AddCommand(sendCmd)
    sendCmd.AddCommand(sendEventCmd)
}
```

### `cmd/send_event.go`

```go
var sendEventCmd = &cobra.Command{
    Use:   "event",
    Short: "Send an event to a Dynatrace platform ingest endpoint",
    Long: `Send an event payload to a Dynatrace platform ingest endpoint.

The payload is user-supplied JSON. dtctl handles authentication and transport.

Examples:
  # Send from file to custom endpoint
  dtctl send event -f activity.json --endpoint agent-activity

  # Send to built-in platform events endpoint (default)
  dtctl send event -f event.json

  # Inline event
  dtctl send event --set event.type=custom --set title="deploy done"

  # Batch send (JSONL, one event per line)
  dtctl send event -f events.jsonl --batch

  # Dry run
  dtctl send event -f event.json --dry-run`,
    Aliases: []string{"events", "ev"},
    RunE:    runSendEvent,
}

func init() {
    sendEventCmd.Flags().StringP("file", "f", "", "Event payload file (- for stdin)")
    sendEventCmd.Flags().String("endpoint", "", "Target custom OpenPipeline endpoint name")
    sendEventCmd.Flags().String("category", "", "Custom endpoint category: sdlc, security (default: events)")
    sendEventCmd.Flags().StringArray("set", []string{}, "Set event fields inline (key=value)")
    sendEventCmd.Flags().Bool("batch", false, "Treat input as JSONL (one event per line)")
    sendEventCmd.Flags().Bool("dry-run", false, "Validate and show payload without sending")
}
```

### `runSendEvent` flow

```
1. LoadConfig()
2. Safety check (OperationCreate — this is a mutating command)
3. Read payload:
   a. If -f: read file or stdin, parse as JSON (or JSONL if --batch)
   b. If --set: build flat JSON object from key=value pairs
   c. If neither: error
4. If --dry-run: print summary and exit
5. NewClientFromConfig(cfg)
6. Resolve endpoint URL from --endpoint + --category
7. POST payload
8. Report result (table or agent envelope)
```

### `pkg/resources/event/send.go`

```go
package event

type SendOptions struct {
    Endpoint string // custom endpoint name, or "" (built-in platform events)
    Category string // "sdlc", "security", or "" (events)
}

type SendResult struct {
    EventsSent int
    Bytes      int
    Endpoint   string
}

// Send posts one or more events to the resolved ingest endpoint.
func Send(client *client.Client, payloads []json.RawMessage, opts SendOptions) (*SendResult, error)

// ResolveURL returns the full ingest URL for the given options.
func ResolveURL(opts SendOptions) string
```

The `Send` function:
1. Resolves the URL via `ResolveURL`
2. For single events: `POST` the JSON body with `Content-Type: application/json`
3. For batch: `POST` newline-delimited JSON
4. Returns `SendResult` on 2xx, wraps API error on non-2xx

---

## Safety & Auth

### Safety checks

`send event` is a **mutating command** — it creates data. It must use the standard safety checker:

```go
checker, err := NewSafetyChecker(cfg)
if err != nil { return err }
if err := checker.CheckError(safety.OperationCreate, safety.OwnershipUnknown); err != nil {
    return err
}
```

Skip for `--dry-run`.

### Token scopes

All scopes use the **platform token format** (`domain:resource:permission`). No classic token scopes.

The required scopes depend on the target endpoint:

| Endpoint | Required scope |
|---|---|
| Built-in platform events | `storage:events:write` |
| Custom OpenPipeline events | `storage:events:write` |
| Custom OpenPipeline security events | `storage:security.events:write` |

> **Note:** The exact scope names for custom OpenPipeline ingest need to be verified against the current platform API documentation before implementation. The scopes listed above follow the existing codebase pattern (`storage:<resource>:write`) but may differ for custom endpoints. Do not invent scope names — confirm them first.

Scope requirements should be documented in `docs/TOKEN_SCOPES.md` and surfaced in error messages when auth fails.

---

## Error Handling

| Scenario | Behavior |
|---|---|
| No `-f` and no `--set` | Error: "provide event payload via -f or --set" |
| Both `-f` and `--set` | Error: "use either -f or --set, not both" |
| Invalid JSON in file | Error: "invalid JSON at line N: ..." |
| `--category` without `--endpoint` | Error: "--category requires --endpoint" |
| HTTP 401/403 | Error with token scope hint |
| HTTP 4xx | Show API error message |
| HTTP 5xx | Retry per standard client policy, then error |
| `--batch` with non-JSONL | Error: "line N is not valid JSON" |

---

## What This Design Intentionally Leaves Out

- **No payload schema validation.** Users send whatever JSON they want. Dynatrace will reject malformed events server-side with a clear error. dtctl only checks that the input is valid JSON.
- **No event construction DSL.** `--set key=value` builds a flat object for convenience. Complex events use `-f`. No nested key syntax, no type coercion beyond simple number/bool detection.
- **No default endpoint from config.** The endpoint is always explicit (or defaults to built-in). This avoids hidden config state and keeps commands self-documenting.
- **No output formatting options.** Success is a one-line confirmation. Errors are structured. There are no tables or YAML views of sent events — the data flows one way.

---

## Future Extensions

These are explicitly out of scope for v1 but the design accommodates them:

- **`send metric`** — POST to platform metrics ingest endpoint
- **`send log`** — POST to platform logs ingest endpoint
- **`--content-type`** — override content type for non-JSON payloads (e.g., metrics line protocol)
- **`--from-query`** — pipe DQL query results as events (composition via shell is already possible: `dtctl query ... -o json | dtctl send event -f -`)

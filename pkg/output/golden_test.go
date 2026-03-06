package output

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

// -update flag: regenerate golden files
// Run: go test ./pkg/output/ -update
var updateGolden = flag.Bool("update", false, "update golden files")

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

func assertGolden(t *testing.T, name string, actual string) {
	t.Helper()
	goldenPath := filepath.Join("testdata", "golden", name+".golden")

	if *updateGolden {
		dir := filepath.Dir(goldenPath)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create golden dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(actual), 0o644); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}
		t.Logf("updated golden file: %s", goldenPath)
		return
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("golden file not found: %s\nRun with -update to create:\n  go test ./pkg/output/ -update\n\nActual output:\n%s", goldenPath, actual)
	}

	if string(expected) != actual {
		t.Errorf("output does not match golden file %s\n\n--- expected ---\n%s\n--- actual ---\n%s\n--- diff hint ---\nRun with -update to accept the new output:\n  go test ./pkg/output/ -update",
			goldenPath, string(expected), actual)
	}
}

// ---------------------------------------------------------------------------
// Test structs mirroring real resource types (with table tags)
// ---------------------------------------------------------------------------

type testWorkflow struct {
	ID          string `json:"id" table:"ID"`
	Title       string `json:"title" table:"TITLE"`
	Owner       string `json:"owner,omitempty" table:"-"`
	Description string `json:"description,omitempty" table:"DESCRIPTION,wide"`
	IsDeployed  bool   `json:"isDeployed" table:"DEPLOYED"`
}

type testSLO struct {
	ID          string `json:"id" table:"ID"`
	Name        string `json:"name" table:"NAME"`
	Description string `json:"description,omitempty" table:"DESCRIPTION,wide"`
}

type testBucket struct {
	BucketName    string `json:"bucketName" table:"NAME"`
	Table         string `json:"table" table:"TABLE"`
	DisplayName   string `json:"displayName" table:"DISPLAY_NAME"`
	Status        string `json:"status" table:"STATUS"`
	RetentionDays int    `json:"retentionDays" table:"RETENTION_DAYS"`
	Updatable     bool   `json:"updatable" table:"UPDATABLE,wide"`
}

type testDocument struct {
	ID        string    `json:"id" table:"ID"`
	Name      string    `json:"name" table:"NAME"`
	Type      string    `json:"type" table:"TYPE"`
	Owner     string    `json:"owner" table:"OWNER"`
	IsPrivate bool      `json:"isPrivate" table:"PRIVATE"`
	Created   time.Time `json:"-" table:"CREATED"`
	Version   int       `json:"version" table:"VERSION,wide"`
}

type testSettingsObject struct {
	SchemaID      string `json:"schemaId" table:"SCHEMA_ID"`
	Summary       string `json:"summary,omitempty" table:"SUMMARY"`
	ObjectIDShort string `json:"-" table:"OBJECT_ID_SHORT"`
	ObjectID      string `json:"objectId" table:"OBJECT_ID,wide"`
	Scope         string `json:"scope" table:"SCOPE,wide"`
}

type testExecution struct {
	ID          string    `json:"id" table:"ID"`
	Workflow    string    `json:"workflow" table:"WORKFLOW"`
	Title       string    `json:"title" table:"TITLE"`
	State       string    `json:"state" table:"STATE"`
	StartedAt   time.Time `json:"startedAt" table:"STARTED"`
	Runtime     int       `json:"runtime,omitempty" table:"RUNTIME"`
	TriggerType string    `json:"triggerType,omitempty" table:"TRIGGER"`
}

// ---------------------------------------------------------------------------
// Test fixtures
// ---------------------------------------------------------------------------

var fixedTime = time.Date(2025, 3, 15, 10, 30, 0, 0, time.UTC)

func workflowFixtures() []testWorkflow {
	return []testWorkflow{
		{ID: "wf-abc123", Title: "Deploy to Production", Owner: "admin", Description: "Deploys latest build to prod", IsDeployed: true},
		{ID: "wf-def456", Title: "Daily Cleanup", Owner: "system", Description: "Removes stale resources", IsDeployed: true},
		{ID: "wf-ghi789", Title: "Incident Response", Owner: "oncall", Description: "", IsDeployed: false},
	}
}

func sloFixtures() []testSLO {
	return []testSLO{
		{ID: "slo-001", Name: "API Availability", Description: "99.9% availability for API endpoints"},
		{ID: "slo-002", Name: "Checkout Latency", Description: "P95 latency < 500ms"},
		{ID: "slo-003", Name: "Error Rate", Description: "Error rate below 0.1%"},
	}
}

func bucketFixtures() []testBucket {
	return []testBucket{
		{BucketName: "default", Table: "logs", DisplayName: "Default Logs", Status: "active", RetentionDays: 35, Updatable: true},
		{BucketName: "custom-metrics", Table: "metrics", DisplayName: "Custom Metrics", Status: "active", RetentionDays: 90, Updatable: true},
		{BucketName: "security-events", Table: "logs", DisplayName: "Security Events", Status: "active", RetentionDays: 365, Updatable: false},
	}
}

func documentFixtures() []testDocument {
	return []testDocument{
		{ID: "doc-aaa111", Name: "Production Overview", Type: "dashboard", Owner: "admin", IsPrivate: false, Created: fixedTime, Version: 3},
		{ID: "doc-bbb222", Name: "Runbook: Incident Response", Type: "notebook", Owner: "oncall", IsPrivate: true, Created: fixedTime.Add(-24 * time.Hour), Version: 1},
		{ID: "doc-ccc333", Name: "Performance Dashboard", Type: "dashboard", Owner: "platform", IsPrivate: false, Created: fixedTime.Add(-72 * time.Hour), Version: 7},
	}
}

func settingsFixtures() []testSettingsObject {
	return []testSettingsObject{
		{SchemaID: "builtin:alerting.profile", Summary: "Default Alerting Profile", ObjectIDShort: "abc123", ObjectID: "vu9U3hXa3q0AAAABABhidWlsdGluOmFsZXJ0aW5nLnByb2ZpbGUABnRlbmFudAAGdGVuYW50ACRhYmMxMjM", Scope: "environment"},
		{SchemaID: "builtin:problem.notifications", Summary: "Email Notification", ObjectIDShort: "def456", ObjectID: "vu9U3hXa3q0AAAABABxidWlsdGluOnByb2JsZW0ubm90aWZpY2F0aW9ucwAGdGVuYW50AAZ0ZW5hbnQAJGRlZjQ1Ng", Scope: "environment"},
		{SchemaID: "builtin:tags.auto-tagging", Summary: "Environment Tag Rule", ObjectIDShort: "ghi789", ObjectID: "vu9U3hXa3q0AAAABABlidWlsdGluOnRhZ3MuYXV0by10YWdnaW5nAAZ0ZW5hbnQABnRlbmFudAAkZ2hpNzg5", Scope: "environment"},
	}
}

func executionFixtures() []testExecution {
	return []testExecution{
		{ID: "exec-001", Workflow: "wf-abc123", Title: "Deploy to Production", State: "SUCCEEDED", StartedAt: fixedTime, Runtime: 45, TriggerType: "Schedule"},
		{ID: "exec-002", Workflow: "wf-abc123", Title: "Deploy to Production", State: "FAILED", StartedAt: fixedTime.Add(-time.Hour), Runtime: 12, TriggerType: "Manual"},
		{ID: "exec-003", Workflow: "wf-def456", Title: "Daily Cleanup", State: "RUNNING", StartedAt: fixedTime.Add(-5 * time.Minute), Runtime: 0, TriggerType: "Schedule"},
	}
}

func dqlRecordsFixture() []map[string]interface{} {
	return []map[string]interface{}{
		{"timestamp": "2025-03-15T10:30:00Z", "host.name": "web-server-01", "status": "ERROR", "content": "Connection timeout to database"},
		{"timestamp": "2025-03-15T10:29:55Z", "host.name": "web-server-02", "status": "WARN", "content": "High memory usage detected"},
		{"timestamp": "2025-03-15T10:29:50Z", "host.name": "api-gateway", "status": "INFO", "content": "Request processed successfully"},
	}
}

func dqlTimeseriesFixture() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"timeframe": map[string]interface{}{
				"start": "2025-03-15T09:00:00Z",
				"end":   "2025-03-15T10:00:00Z",
			},
			"interval": 300000000000, // 5 min in nanoseconds
			"avg(dt.host.cpu.usage)": []interface{}{
				12.5, 15.3, 18.7, 22.1, 35.6,
				42.3, 38.9, 25.4, 19.8, 14.2,
				11.1, 13.7,
			},
		},
	}
}

// ---------------------------------------------------------------------------
// Golden tests: text formats (table, wide, json, yaml, csv)
// ---------------------------------------------------------------------------

func TestGolden_GetWorkflows(t *testing.T) {
	workflows := workflowFixtures()

	formats := map[string]string{
		"table": "table",
		"wide":  "wide",
		"json":  "json",
		"yaml":  "yaml",
		"csv":   "csv",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.PrintList(workflows); err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}
			assertGolden(t, "get/workflows-"+name, buf.String())
		})
	}
}

func TestGolden_GetSLOs(t *testing.T) {
	slos := sloFixtures()

	formats := map[string]string{
		"table": "table",
		"wide":  "wide",
		"json":  "json",
		"yaml":  "yaml",
		"csv":   "csv",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.PrintList(slos); err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}
			assertGolden(t, "get/slos-"+name, buf.String())
		})
	}
}

func TestGolden_GetBuckets(t *testing.T) {
	buckets := bucketFixtures()

	formats := map[string]string{
		"table": "table",
		"wide":  "wide",
		"json":  "json",
		"yaml":  "yaml",
		"csv":   "csv",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.PrintList(buckets); err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}
			assertGolden(t, "get/buckets-"+name, buf.String())
		})
	}
}

func TestGolden_GetDocuments(t *testing.T) {
	docs := documentFixtures()

	formats := map[string]string{
		"table": "table",
		"wide":  "wide",
		"json":  "json",
		"yaml":  "yaml",
		"csv":   "csv",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.PrintList(docs); err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}
			assertGolden(t, "get/documents-"+name, buf.String())
		})
	}
}

func TestGolden_GetSettings(t *testing.T) {
	settings := settingsFixtures()

	formats := map[string]string{
		"table": "table",
		"wide":  "wide",
		"json":  "json",
		"yaml":  "yaml",
		"csv":   "csv",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.PrintList(settings); err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}
			assertGolden(t, "get/settings-"+name, buf.String())
		})
	}
}

func TestGolden_GetExecutions(t *testing.T) {
	executions := executionFixtures()

	formats := map[string]string{
		"table": "table",
		"json":  "json",
		"yaml":  "yaml",
		"csv":   "csv",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.PrintList(executions); err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}
			assertGolden(t, "get/executions-"+name, buf.String())
		})
	}
}

// ---------------------------------------------------------------------------
// Golden tests: describe (single-object Print)
// ---------------------------------------------------------------------------

func TestGolden_DescribeWorkflow(t *testing.T) {
	wf := workflowFixtures()[0]

	formats := map[string]string{
		"table": "table",
		"json":  "json",
		"yaml":  "yaml",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.Print(wf); err != nil {
				t.Fatalf("Print failed: %v", err)
			}
			assertGolden(t, "describe/workflow-"+name, buf.String())
		})
	}
}

func TestGolden_DescribeBucket(t *testing.T) {
	bucket := bucketFixtures()[0]

	formats := map[string]string{
		"table": "table",
		"json":  "json",
		"yaml":  "yaml",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.Print(bucket); err != nil {
				t.Fatalf("Print failed: %v", err)
			}
			assertGolden(t, "describe/bucket-"+name, buf.String())
		})
	}
}

// ---------------------------------------------------------------------------
// Golden tests: DQL query results (map-based records)
// ---------------------------------------------------------------------------

func TestGolden_QueryDQL(t *testing.T) {
	records := dqlRecordsFixture()

	formats := map[string]string{
		"table": "table",
		"json":  "json",
		"csv":   "csv",
	}

	for name, format := range formats {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			printer := NewPrinterWithWriter(format, &buf)
			if err := printer.PrintList(records); err != nil {
				t.Fatalf("PrintList failed: %v", err)
			}
			assertGolden(t, "query/dql-"+name, buf.String())
		})
	}
}

// ---------------------------------------------------------------------------
// Golden tests: empty results
// ---------------------------------------------------------------------------

func TestGolden_EmptyResults(t *testing.T) {
	t.Run("table", func(t *testing.T) {
		var buf bytes.Buffer
		printer := NewPrinterWithWriter("table", &buf)
		if err := printer.PrintList([]testWorkflow{}); err != nil {
			t.Fatalf("PrintList failed: %v", err)
		}
		assertGolden(t, "empty/workflows-table", buf.String())
	})

	t.Run("json", func(t *testing.T) {
		var buf bytes.Buffer
		printer := NewPrinterWithWriter("json", &buf)
		if err := printer.PrintList([]testWorkflow{}); err != nil {
			t.Fatalf("PrintList failed: %v", err)
		}
		assertGolden(t, "empty/workflows-json", buf.String())
	})
}

// ---------------------------------------------------------------------------
// Golden tests: visual formats (chart, sparkline, barchart, braille)
// Use fixed dimensions for deterministic output.
// ---------------------------------------------------------------------------

func TestGolden_QueryDQL_Chart(t *testing.T) {
	records := dqlTimeseriesFixture()

	var buf bytes.Buffer
	printer := NewPrinterWithOpts(PrinterOptions{
		Format: "chart",
		Writer: &buf,
		Width:  80,
		Height: 15,
	})
	if err := printer.PrintList(records); err != nil {
		t.Fatalf("PrintList failed: %v", err)
	}
	// Strip ANSI for deterministic comparison
	assertGolden(t, "query/dql-chart", stripANSI(buf.String()))
}

func TestGolden_QueryDQL_Sparkline(t *testing.T) {
	records := dqlTimeseriesFixture()

	var buf bytes.Buffer
	printer := NewPrinterWithOpts(PrinterOptions{
		Format: "sparkline",
		Writer: &buf,
		Width:  60,
	})
	if err := printer.PrintList(records); err != nil {
		t.Fatalf("PrintList failed: %v", err)
	}
	assertGolden(t, "query/dql-sparkline", stripANSI(buf.String()))
}

func TestGolden_QueryDQL_BarChart(t *testing.T) {
	records := dqlTimeseriesFixture()

	var buf bytes.Buffer
	printer := NewPrinterWithOpts(PrinterOptions{
		Format: "barchart",
		Writer: &buf,
		Width:  60,
	})
	if err := printer.PrintList(records); err != nil {
		t.Fatalf("PrintList failed: %v", err)
	}
	assertGolden(t, "query/dql-barchart", stripANSI(buf.String()))
}

func TestGolden_QueryDQL_Braille(t *testing.T) {
	records := dqlTimeseriesFixture()

	var buf bytes.Buffer
	printer := NewPrinterWithOpts(PrinterOptions{
		Format: "braille",
		Writer: &buf,
		Width:  40,
		Height: 10,
	})
	if err := printer.PrintList(records); err != nil {
		t.Fatalf("PrintList failed: %v", err)
	}
	assertGolden(t, "query/dql-braille", stripANSI(buf.String()))
}

// ---------------------------------------------------------------------------
// Golden tests: error output
// ---------------------------------------------------------------------------

func TestGolden_ErrorNotFound(t *testing.T) {
	// Simulate the error message a user would see
	errMsg := "Error: workflow \"my-workflow\" not found\n"
	assertGolden(t, "errors/not-found", errMsg)
}

func TestGolden_ErrorAuth(t *testing.T) {
	errMsg := "Error: authentication failed: token expired or invalid\n"
	assertGolden(t, "errors/auth-error", errMsg)
}

func TestGolden_ErrorPermission(t *testing.T) {
	errMsg := "Error: insufficient permissions: requires scope \"automation:workflows:read\"\n"
	assertGolden(t, "errors/permission-error", errMsg)
}

// ---------------------------------------------------------------------------
// Golden tests: agent mode output
// ---------------------------------------------------------------------------

func TestGolden_AgentMode(t *testing.T) {
	workflows := workflowFixtures()

	t.Run("list", func(t *testing.T) {
		var buf bytes.Buffer
		ctx := &ResponseContext{
			Verb:     "get",
			Resource: "workflow",
		}
		printer := NewAgentPrinter(&buf, ctx)
		printer.SetTotal(len(workflows))
		printer.SetSuggestions([]string{
			"Run 'dtctl describe workflow <id>' for details",
			"Run 'dtctl exec workflow <id>' to trigger a workflow",
		})
		if err := printer.PrintList(workflows); err != nil {
			t.Fatalf("PrintList failed: %v", err)
		}
		assertGolden(t, "get/workflows-agent", buf.String())
	})
}

// ---------------------------------------------------------------------------
// Golden tests: watch output (change prefixes)
// ---------------------------------------------------------------------------

func TestGolden_WatchChanges(t *testing.T) {
	wfs := workflowFixtures()

	changes := []Change{
		{Type: ChangeTypeAdded, Resource: wfs[0]},
		{Type: ChangeTypeModified, Resource: wfs[1]},
		{Type: ChangeTypeDeleted, Resource: wfs[2]},
	}

	var buf bytes.Buffer
	basePrinter := NewPrinterWithWriter("table", &buf)
	watchPrinter := NewWatchPrinterWithWriter(basePrinter, &buf, false) // no color for deterministic output

	if err := watchPrinter.PrintChanges(changes); err != nil {
		t.Fatalf("PrintChanges failed: %v", err)
	}

	assertGolden(t, "get/workflows-watch-changes", buf.String())
}

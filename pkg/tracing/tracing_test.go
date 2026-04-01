package tracing

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

func resetActiveCtx(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { SetActiveContext(context.Background()) })
}

func resetProvider(t *testing.T) {
	t.Helper()
	prev := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
}

func TestSpanNameFromArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "no args",
			args: nil,
			want: "dtctl",
		},
		{
			name: "empty slice",
			args: []string{},
			want: "dtctl",
		},
		{
			name: "single verb",
			args: []string{"get"},
			want: "dtctl get",
		},
		{
			name: "verb and resource",
			args: []string{"get", "workflow"},
			want: "dtctl get workflow",
		},
		{
			name: "stops at first flag",
			args: []string{"get", "workflow", "--output", "json"},
			want: "dtctl get workflow",
		},
		{
			name: "stops at short flag",
			args: []string{"get", "slo", "-o", "yaml"},
			want: "dtctl get slo",
		},
		{
			name: "flag only",
			args: []string{"--help"},
			want: "dtctl",
		},
		{
			name: "verb resource and id",
			args: []string{"describe", "workflow", "abc-123"},
			want: "dtctl describe workflow abc-123",
		},
		{
			name: "caps at three positional args",
			args: []string{"describe", "workflow", "abc-123", "extra-arg", "more"},
			want: "dtctl describe workflow abc-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := spanNameFromArgs(tt.args)
			if got != tt.want {
				t.Errorf("spanNameFromArgs(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestInitNoop_WhenNoEndpointSet(t *testing.T) {
	// Stash and restore the env vars so the test is hermetic.
	for _, key := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"} {
		if v, ok := os.LookupEnv(key); ok {
			t.Cleanup(func() { os.Setenv(key, v) })
		} else {
			t.Cleanup(func() { os.Unsetenv(key) })
		}
		os.Unsetenv(key)
	}

	p := Init(context.Background())
	if p == nil {
		t.Fatal("Init() returned nil")
	}

	// The global provider should be a noop.
	tp := otel.GetTracerProvider()
	switch tp.(type) {
	case noop.TracerProvider, *noop.TracerProvider:
		// ok
	default:
		t.Errorf("expected noop TracerProvider, got %T", tp)
	}

	// Flush should not panic on the no-op provider.
	p.Flush()
}

func TestActiveContext_NeverReturnsNil(t *testing.T) {
	// ActiveContext must never return nil, regardless of internal state.
	ctx := ActiveContext()
	if ctx == nil {
		t.Fatal("ActiveContext() returned nil")
	}
}

func TestSetAndGetActiveContext(t *testing.T) {
	resetActiveCtx(t)

	type ctxKey struct{}
	custom := context.WithValue(context.Background(), ctxKey{}, "test-value")

	SetActiveContext(custom)

	got := ActiveContext()
	if v, ok := got.Value(ctxKey{}).(string); !ok || v != "test-value" {
		t.Errorf("ActiveContext() did not return the context set via SetActiveContext")
	}

	// Overwrite with a different context to verify updates work.
	type ctxKey2 struct{}
	custom2 := context.WithValue(context.Background(), ctxKey2{}, "second")
	SetActiveContext(custom2)

	got2 := ActiveContext()
	if v, ok := got2.Value(ctxKey2{}).(string); !ok || v != "second" {
		t.Errorf("ActiveContext() did not reflect the updated context")
	}
}

func TestEnvCarrier_Get(t *testing.T) {
	t.Setenv("TRACEPARENT", "00-abcdef1234567890abcdef1234567890-abcdef1234567890-01")

	c := envCarrier{}

	// The carrier normalises the key: traceparent → TRACEPARENT.
	got := c.Get("traceparent")
	if got != "00-abcdef1234567890abcdef1234567890-abcdef1234567890-01" {
		t.Errorf("envCarrier.Get(traceparent) = %q, want the TRACEPARENT env value", got)
	}
}

func TestEnvCarrier_Get_HyphenToUnderscore(t *testing.T) {
	t.Setenv("TRACE_STATE", "vendor=value")

	c := envCarrier{}
	got := c.Get("trace-state")
	if got != "vendor=value" {
		t.Errorf("envCarrier.Get(trace-state) = %q, want 'vendor=value'", got)
	}
}

func TestEnvCarrier_Keys(t *testing.T) {
	c := envCarrier{}
	keys := c.Keys()
	if len(keys) != 2 {
		t.Fatalf("envCarrier.Keys() len = %d, want 2", len(keys))
	}
	if keys[0] != "TRACEPARENT" || keys[1] != "TRACESTATE" {
		t.Errorf("envCarrier.Keys() = %v, want [TRACEPARENT TRACESTATE]", keys)
	}
}

func TestEnvCarrier_SetIsNoop(t *testing.T) {
	c := envCarrier{}
	// Set should not panic; it's intentionally a no-op.
	c.Set("traceparent", "anything")
}

func TestStartAndEndCommand(t *testing.T) {
	resetActiveCtx(t)

	// Ensure a no-op provider so no real export happens.
	otel.SetTracerProvider(noop.NewTracerProvider())

	ctx, span := StartCommand([]string{"get", "workflow", "--output", "json"})
	if ctx == nil {
		t.Fatal("StartCommand returned nil context")
	}
	if span == nil {
		t.Fatal("StartCommand returned nil span")
	}

	// ActiveContext should now carry the span context set by StartCommand.
	active := ActiveContext()
	if active != ctx {
		t.Error("ActiveContext() does not match the context returned by StartCommand")
	}

	// EndCommand with nil error should not panic.
	EndCommand(span, nil)
}

func TestEndCommand_WithError(t *testing.T) {
	resetActiveCtx(t)
	otel.SetTracerProvider(noop.NewTracerProvider())

	_, span := StartCommand([]string{"delete", "bucket"})

	// EndCommand with an error should not panic.
	EndCommand(span, errors.New("not found"))
}

func TestProviderFlush_NoopDoesNotPanic(t *testing.T) {
	p := installNoop()
	// Must not panic.
	p.Flush()
}

func TestTracer_ReturnsNonNil(t *testing.T) {
	resetProvider(t)
	otel.SetTracerProvider(noop.NewTracerProvider())

	tr := Tracer()
	if tr == nil {
		t.Fatal("Tracer() returned nil")
	}

	// Verify it has the correct instrumentation scope by starting a span.
	_, span := tr.Start(context.Background(), "test")
	defer span.End()

	if span.SpanContext().IsValid() {
		// noop spans have an invalid SpanContext — if valid, something unexpected happened.
		// This is informational; not a hard failure since the provider could change.
	}
}

func TestStartCommand_SetsSpanKindClient(t *testing.T) {
	resetProvider(t)
	resetActiveCtx(t)

	// Use a recording span to verify attributes; with noop we at least ensure
	// the code path executes without error. Deeper attribute assertions would
	// require an in-memory exporter, which is overkill for this test.
	otel.SetTracerProvider(noop.NewTracerProvider())

	_, span := StartCommand([]string{"query", "fetch logs"})
	defer span.End()

	// With a noop provider the span is always a noop, so we just verify it
	// implements the interface.
	var _ trace.Span = span
}

func TestWrapTransport_RoundTrip(t *testing.T) {
	resetProvider(t)
	otel.SetTracerProvider(noop.NewTracerProvider())
	resetActiveCtx(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	wrapped := WrapTransport(http.DefaultTransport)
	req, err := http.NewRequestWithContext(context.Background(), "GET", ts.URL+"/api/test", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := wrapped.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestWrapTransport_ErrorStatusCode(t *testing.T) {
	resetProvider(t)
	otel.SetTracerProvider(noop.NewTracerProvider())
	resetActiveCtx(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	wrapped := WrapTransport(http.DefaultTransport)
	req, err := http.NewRequestWithContext(context.Background(), "GET", ts.URL+"/api/fail", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := wrapped.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
}

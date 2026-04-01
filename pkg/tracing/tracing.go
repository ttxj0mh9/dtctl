// Package tracing provides OpenTelemetry trace instrumentation for dtctl.
//
// Tracing is opt-in: nothing happens unless OTEL_EXPORTER_OTLP_ENDPOINT or
// OTEL_EXPORTER_OTLP_TRACES_ENDPOINT is set. Errors during SDK setup are
// silenced — tracing is strictly best-effort for a CLI tool.
package tracing

import (
	"context"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/dynatrace-oss/dtctl/pkg/version"
)

const instrumentationScope = "github.com/dynatrace-oss/dtctl"

var activeCtx atomic.Value // stores *ctxHolder

// ctxHolder wraps a context.Context so that atomic.Value always stores the
// same concrete type, avoiding the "inconsistently typed value" panic.
type ctxHolder struct {
	ctx context.Context
}

// SetActiveContext stores the trace context so the HTTP client can propagate it
// into outgoing requests, making HTTP spans children of the root command span.
func SetActiveContext(ctx context.Context) {
	activeCtx.Store(&ctxHolder{ctx: ctx})
}

// ActiveContext returns the stored trace context, or context.Background() if none.
func ActiveContext() context.Context {
	if h, ok := activeCtx.Load().(*ctxHolder); ok && h != nil {
		return h.ctx
	}
	return context.Background()
}

// Provider wraps a TracerProvider and provides lifecycle management.
type Provider struct {
	shutdown func(context.Context) error
}

// Flush forces a flush of pending spans and shuts down the exporter, with a
// 5-second timeout. Intended to be called once before process exit.
func (p *Provider) Flush() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = p.shutdown(ctx)
}

// Init sets up the global OTel tracer provider with an OTLP HTTP exporter.
// Returns a no-op provider (with negligible overhead) if no OTLP endpoint is
// configured via environment variables.
func Init(ctx context.Context) *Provider {
	_, hasEndpoint := os.LookupEnv("OTEL_EXPORTER_OTLP_ENDPOINT")
	_, hasTracesEndpoint := os.LookupEnv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")
	if !hasEndpoint && !hasTracesEndpoint {
		return installNoop()
	}

	exp, err := otlptracehttp.New(ctx)
	if err != nil {
		logrus.Debugf("tracing: OTLP exporter init failed: %v", err)
		return installNoop()
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("dtctl"),
			semconv.ServiceVersion(version.Version),
		),
	)
	if err != nil || res == nil {
		res = resource.Default()
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Provider{shutdown: tp.Shutdown}
}

// Tracer returns the global dtctl tracer.
func Tracer() trace.Tracer {
	return otel.GetTracerProvider().Tracer(instrumentationScope)
}

// envCarrier implements propagation.TextMapCarrier over environment variables.
// The W3C Trace Context convention for CLI tools is to read TRACEPARENT and
// TRACESTATE so that a parent process can propagate its trace into the child.
type envCarrier struct{}

func (envCarrier) Get(key string) string {
	return os.Getenv(strings.ToUpper(strings.ReplaceAll(key, "-", "_")))
}

func (envCarrier) Set(_, _ string) {} // read-only

func (envCarrier) Keys() []string {
	return []string{"TRACEPARENT", "TRACESTATE"}
}

// StartCommand creates a root span for the CLI command being executed.
// The span name is derived from the CLI arguments, e.g. "dtctl get workflow".
//
// If TRACEPARENT (and optionally TRACESTATE) are set in the environment the
// new span is created as a child of that incoming trace, allowing a parent
// process or CI pipeline to propagate its trace context into dtctl.
//
// The span context is also stored so the HTTP transport can use it as the
// parent for every outgoing API request.
func StartCommand(args []string) (context.Context, trace.Span) {
	name := spanNameFromArgs(args)

	// Extract any incoming W3C trace context from environment variables.
	parent := otel.GetTextMapPropagator().Extract(context.Background(), envCarrier{})

	ctx, span := Tracer().Start(parent, name,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("dtctl.command", name),
		),
	)
	SetActiveContext(ctx)
	return ctx, span
}

// EndCommand ends the root command span. If err is non-nil the span is marked
// as an error before being closed.
func EndCommand(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End()
}

// installNoop registers a no-op global provider and returns a stub Provider.
func installNoop() *Provider {
	otel.SetTracerProvider(noop.NewTracerProvider())
	return &Provider{shutdown: func(context.Context) error { return nil }}
}

// spanNameFromArgs builds a human-readable span name from CLI args, stopping
// at the first flag or after 3 positional args to keep span names bounded,
// e.g. ["get", "workflow", "--output", "json"] → "dtctl get workflow".
func spanNameFromArgs(args []string) string {
	const maxPositional = 3
	parts := []string{"dtctl"}
	for _, a := range args {
		if strings.HasPrefix(a, "-") || len(parts) > maxPositional {
			break
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " ")
}

// WrapTransport wraps an http.RoundTripper with trace instrumentation.
// Each outgoing request creates a child span under the active command trace.
// This is a lightweight alternative to otelhttp that avoids the contrib
// dependency and its transitive gRPC/metric imports.
func WrapTransport(base http.RoundTripper) http.RoundTripper {
	return &tracingTransport{base: base}
}

type tracingTransport struct {
	base http.RoundTripper
}

func (t *tracingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, span := Tracer().Start(req.Context(), req.Method,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			semconv.HTTPRequestMethodKey.String(req.Method),
			attribute.String("url.path", req.URL.Path),
		),
	)
	defer span.End()

	// Clone the request to avoid mutating the caller's original (the
	// RoundTripper contract forbids modifying the provided *Request).
	req = req.Clone(ctx)

	// Inject W3C trace context headers (traceparent/tracestate) so the
	// receiving service can correlate this request with the dtctl trace.
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return resp, err
	}

	span.SetAttributes(semconv.HTTPResponseStatusCode(resp.StatusCode))
	if resp.StatusCode >= 400 {
		span.SetStatus(codes.Error, http.StatusText(resp.StatusCode))
	}
	return resp, nil
}

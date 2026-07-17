// Package tracing implements the H1 distributed-tracing baseline. Tracing is
// opt-in per deployment: when OTEL_EXPORTER_OTLP_ENDPOINT is set (the
// standard OTel env var, e.g. http://otel-collector:4318), every service
// exports OTLP/HTTP spans for its inbound requests and propagates W3C trace
// context; when unset, Init is a no-op and WrapHandler adds zero overhead
// beyond a header check. The gateway forwards traceparent untouched (it only
// strips X-Service-* headers), so a request is one trace across services.
package tracing

import (
	"context"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.uber.org/zap"
)

// enabled reports whether an OTLP endpoint is configured for this process.
func enabled() bool { return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" }

// Init installs the global tracer provider and W3C propagators. It returns a
// shutdown func to defer in main; both are safe no-ops when tracing is
// disabled. Exporter construction never blocks startup: OTLP/HTTP dials
// lazily on first export.
func Init(ctx context.Context, serviceName string, logger *zap.Logger) func(context.Context) error {
	if !enabled() {
		return func(context.Context) error { return nil }
	}
	exp, err := otlptracehttp.New(ctx)
	if err != nil {
		logger.Warn("OTel exporter init failed; tracing disabled", zap.Error(err))
		return func(context.Context) error { return nil }
	}
	res, _ := sdkresource.Merge(sdkresource.Default(),
		sdkresource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName(serviceName)))
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp, sdktrace.WithBatchTimeout(5*time.Second)),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))
	logger.Info("OTel tracing enabled",
		zap.String("endpoint", os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")))
	return tp.Shutdown
}

// WrapHandler instruments an HTTP handler with per-request server spans named
// by method+path. Health and metrics probes are not traced.
func WrapHandler(h http.Handler, serviceName string) http.Handler {
	if !enabled() {
		return h
	}
	return otelhttp.NewHandler(h, serviceName,
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method + " " + r.URL.Path
		}),
		otelhttp.WithFilter(func(r *http.Request) bool {
			return r.URL.Path != "/actuator/health" && r.URL.Path != "/metrics"
		}),
	)
}

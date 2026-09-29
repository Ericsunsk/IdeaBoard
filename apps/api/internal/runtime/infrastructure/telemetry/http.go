package telemetry

import (
	"context"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/gin-gonic/gin"
)

const tracerName = "ideaboard/generation"

const maxAttributeStringLength = 128

// HTTPMiddleware creates a best-effort server span and propagates W3C
// Trace Context. The API tracer is a no-op until an SDK provider is wired,
// so telemetry availability never becomes a business-path dependency.
func HTTPMiddleware() gin.HandlerFunc {
	tracer := otel.Tracer(tracerName)
	propagator := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	)
	return func(c *gin.Context) {
		if c == nil || c.Request == nil {
			return
		}
		parent := propagator.Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		name := strings.TrimSpace(c.Request.Method)
		if name == "" {
			name = "HTTP"
		}
		ctx, span := tracer.Start(parent, name, trace.WithSpanKind(trace.SpanKindServer))
		c.Request = c.Request.WithContext(ctx)
		defer func() {
			status := c.Writer.Status()
			if status >= http.StatusInternalServerError {
				span.SetStatus(codes.Error, "http server error")
			}
			span.End()
		}()
		c.Next()
	}
}

// Start creates a generation span with the low-cardinality attributes that
// are safe to export. Callers pass business facts only; prompts, identifiers,
// URLs, payloads, SQL values, and credentials have no representation here.
func Start(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	tracer := otel.Tracer(tracerName)
	spanCtx, span := tracer.Start(ctx, strings.TrimSpace(name))
	span.SetAttributes(allowlisted(attrs...)...)
	return spanCtx, span
}

// StartLinked starts a worker span and links it to the producer span carried
// across an asynchronous queue boundary.
func StartLinked(ctx context.Context, parent string, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	parentCtx := Extract(ctx, parent)
	link := trace.WithLinks(trace.Link{SpanContext: Context(parentCtx)})
	spanCtx, span := otel.Tracer(tracerName).Start(ctx, strings.TrimSpace(name), link)
	span.SetAttributes(allowlisted(attrs...)...)
	return spanCtx, span
}

// Inject returns the W3C traceparent value for minimal queue metadata.
func Inject(ctx context.Context) string {
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	return carrier.Get("traceparent")
}

// Extract restores a W3C traceparent into ctx. Invalid or absent metadata is
// ignored and leaves the context unchanged.
func Extract(ctx context.Context, parent string) context.Context {
	if strings.TrimSpace(parent) == "" {
		return ctx
	}
	carrier := propagation.MapCarrier{"traceparent": strings.TrimSpace(parent)}
	return propagation.TraceContext{}.Extract(ctx, carrier)
}

func allowlisted(attrs ...attribute.KeyValue) []attribute.KeyValue {
	allowed := map[attribute.Key]struct{}{
		"service": {}, "resolution": {}, "pool_version": {},
		"selected_model": {}, "attempt_number": {}, "retry_layer": {},
		"failure_class": {}, "queue": {},
	}
	out := make([]attribute.KeyValue, 0, len(attrs))
	for _, attr := range attrs {
		if _, ok := allowed[attr.Key]; ok {
			if attr.Value.Type() == attribute.STRING {
				value := strings.TrimSpace(attr.Value.AsString())
				if len(value) > maxAttributeStringLength {
					value = value[:maxAttributeStringLength]
				}
				attr = attribute.String(string(attr.Key), value)
			}
			out = append(out, attr)
		}
	}
	return out
}

// End marks a span failed without allowing telemetry errors to escape the
// business path.
func End(span trace.Span, err error) {
	if span == nil {
		return
	}
	if err != nil {
		span.SetStatus(codes.Error, "operation failed")
	}
	span.End()
}

// Context returns the active generation span context for linking worker
// spans without exposing arbitrary request data as trace attributes.
func Context(ctx context.Context) trace.SpanContext {
	if ctx == nil {
		return trace.SpanContext{}
	}
	return trace.SpanContextFromContext(ctx)
}

package telemetry

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func TestTraceParentInjectExtract(t *testing.T) {
	traceID, err := trace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
	if err != nil {
		t.Fatal(err)
	}
	spanID, err := trace.SpanIDFromHex("0102030405060708")
	if err != nil {
		t.Fatal(err)
	}
	original := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})

	parent := trace.ContextWithSpanContext(context.Background(), original)
	traceparent := Inject(parent)
	if traceparent == "" {
		t.Fatal("Inject returned an empty traceparent")
	}
	restored := Context(Extract(context.Background(), traceparent))
	if !restored.IsValid() || restored.TraceID() != original.TraceID() || restored.SpanID() != original.SpanID() {
		t.Fatalf("Extract restored %v, want %v", restored, original)
	}
}

func TestAllowlistedFiltersUnsafeAttributes(t *testing.T) {
	longService := "  " + string(make([]byte, maxAttributeStringLength+10))
	attrs := allowlisted(
		attribute.String("service", longService),
		attribute.String("prompt", "must never be exported"),
		attribute.String("user_id", "42"),
		attribute.Int("attempt_number", 2),
	)
	if len(attrs) != 2 {
		t.Fatalf("allowlisted returned %d attributes, want 2", len(attrs))
	}
	if attrs[0].Key != "service" || attrs[1].Key != "attempt_number" {
		t.Fatalf("allowlisted returned keys %q and %q", attrs[0].Key, attrs[1].Key)
	}
	if len(attrs[0].Value.AsString()) != maxAttributeStringLength {
		t.Fatalf("service attribute length = %d, want %d", len(attrs[0].Value.AsString()), maxAttributeStringLength)
	}
}

func TestNoopTracingIsBestEffort(t *testing.T) {
	ctx, span := Start(context.Background(), "generation.test", attribute.String("prompt", "filtered"))
	if ctx == nil || span == nil {
		t.Fatal("Start must return a usable no-op span")
	}
	End(span, context.Canceled)

	ctx, span = StartLinked(ctx, "invalid", "generation.test.linked")
	if ctx == nil || span == nil {
		t.Fatal("StartLinked must tolerate invalid parent metadata")
	}
	End(span, nil)
}

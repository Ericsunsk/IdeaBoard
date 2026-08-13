package worker

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestProviderBatchOrchestratorFillsMissingOutputsWithoutExceedingTwoCallsPerSlot(t *testing.T) {
	var requests []int
	orchestrator := newProviderBatchOrchestrator(func(_ context.Context, request providerRequest) (providerCallResult, error) {
		requests = append(requests, request.OutputCount)
		if len(requests) == 1 {
			return providerCallResult{ImageURLs: []string{"a", "b"}}, nil
		}
		return providerCallResult{ImageURLs: []string{string(rune('a' + len(requests)))}}, nil
	})
	orchestrator.wait = func(context.Context, time.Duration) error { return nil }

	got, err := orchestrator.run(context.Background(), providerRequest{OutputCount: 4})
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if !reflect.DeepEqual(got.ImageURLs, []string{"a", "b", "c", "d"}) {
		t.Fatalf("image urls = %#v", got.ImageURLs)
	}
	if !reflect.DeepEqual(requests, []int{4, 1, 1}) {
		t.Fatalf("request counts = %#v", requests)
	}
}

func TestProviderBatchOrchestratorKeepsPartialSuccessOnPermanentFillFailure(t *testing.T) {
	calls := 0
	orchestrator := newProviderBatchOrchestrator(func(_ context.Context, request providerRequest) (providerCallResult, error) {
		calls++
		if calls == 1 {
			return providerCallResult{ImageURLs: []string{"a", "b"}}, nil
		}
		return providerCallResult{}, newUpstreamHTTPError(400, "invalid")
	})

	got, err := orchestrator.run(context.Background(), providerRequest{OutputCount: 4})
	if err != nil {
		t.Fatalf("partial run error: %v", err)
	}
	if !reflect.DeepEqual(got.ImageURLs, []string{"a", "b"}) {
		t.Fatalf("image urls = %#v", got.ImageURLs)
	}
}

func TestProviderBatchOrchestratorRetriesTransientFillOnce(t *testing.T) {
	calls := 0
	orchestrator := newProviderBatchOrchestrator(func(_ context.Context, request providerRequest) (providerCallResult, error) {
		calls++
		if calls == 1 {
			return providerCallResult{ImageURLs: []string{"a"}}, nil
		}
		if calls == 2 {
			return providerCallResult{}, newUpstreamHTTPError(500, "temporary")
		}
		return providerCallResult{ImageURLs: []string{"b"}}, nil
	})
	orchestrator.wait = func(context.Context, time.Duration) error { return nil }

	got, err := orchestrator.run(context.Background(), providerRequest{OutputCount: 2})
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if !reflect.DeepEqual(got.ImageURLs, []string{"a", "b"}) || calls != 3 {
		t.Fatalf("urls=%#v calls=%d", got.ImageURLs, calls)
	}
}

func TestProviderBatchOrchestratorPreservesRepeatedProviderURLsAsDistinctOutputs(t *testing.T) {
	calls := 0
	orchestrator := newProviderBatchOrchestrator(func(_ context.Context, request providerRequest) (providerCallResult, error) {
		calls++
		if calls == 1 {
			return providerCallResult{ImageURLs: []string{"a", "a"}}, nil
		}
		return providerCallResult{ImageURLs: []string{"b"}}, nil
	})
	orchestrator.wait = func(context.Context, time.Duration) error { return nil }

	got, err := orchestrator.run(context.Background(), providerRequest{OutputCount: 3})
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if !reflect.DeepEqual(got.ImageURLs, []string{"a", "a", "b"}) {
		t.Fatalf("image urls = %#v", got.ImageURLs)
	}
}

func TestProviderRetryDelayUsesBoundedJitter(t *testing.T) {
	delay := providerRetryDelay(errors.New("temporary"))
	if delay < 125*time.Millisecond || delay > 250*time.Millisecond {
		t.Fatalf("delay=%s, want within [125ms, 250ms]", delay)
	}
}

func TestProviderRetryDelayPrefersRetryAfter(t *testing.T) {
	want := 3 * time.Second
	got := providerRetryDelay(&upstreamHTTPError{retryAfter: want})
	if got != want {
		t.Fatalf("retry delay=%s, want Retry-After %s", got, want)
	}
}

func TestProviderBatchOrchestratorStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	orchestrator := newProviderBatchOrchestrator(func(context.Context, providerRequest) (providerCallResult, error) {
		return providerCallResult{}, errors.New("must not call provider")
	})
	if _, err := orchestrator.run(ctx, providerRequest{OutputCount: 2}); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}

func TestProviderBatchOrchestratorRejectsInvalidOutputCount(t *testing.T) {
	orchestrator := newProviderBatchOrchestrator(func(context.Context, providerRequest) (providerCallResult, error) {
		return providerCallResult{}, errors.New("must not call provider")
	})
	if _, err := orchestrator.run(context.Background(), providerRequest{OutputCount: 0}); err == nil {
		t.Fatal("expected invalid output_count error")
	}
}

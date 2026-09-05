package providers

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

type providerBatchCall func(context.Context, providerRequest) (providerCallResult, error)

type providerBatchOrchestrator struct {
	call providerBatchCall
	wait func(context.Context, time.Duration) error
}

func newProviderBatchOrchestrator(call providerBatchCall) providerBatchOrchestrator {
	return providerBatchOrchestrator{call: call, wait: waitProviderRetry}
}

func (o providerBatchOrchestrator) run(ctx context.Context, request providerRequest) (providerCallResult, error) {
	requested := request.OutputCount
	if requested < 1 || requested > 4 {
		return providerCallResult{}, fmt.Errorf("output_count 必须是 1-4 的整数")
	}
	maxCalls := requested * 2
	callCount := 0
	result := providerCallResult{}

	call := func(current providerRequest) (providerCallResult, error) {
		if err := ctx.Err(); err != nil {
			return providerCallResult{}, err
		}
		callCount++
		return o.call(ctx, current)
	}

	nativeResult, err := call(request)
	if err != nil && shouldFailoverProviderError(err) && callCount < maxCalls {
		if waitErr := o.wait(ctx, providerRetryDelay(err)); waitErr != nil {
			return providerCallResult{}, waitErr
		}
		nativeResult, err = call(request)
	}
	if err != nil {
		return nativeResult, err
	}
	mergeProviderBatchResult(&result, nativeResult, requested)

	lastErr, err := o.fillProviderBatch(ctx, request, requested, maxCalls, &callCount, &result, call)
	if err != nil {
		return providerCallResult{}, err
	}

	if len(result.ImageURLs) == 0 {
		if lastErr != nil {
			return providerCallResult{}, lastErr
		}
		return providerCallResult{}, fmt.Errorf("API 未返回图片")
	}
	result.ImageURL = result.ImageURLs[0]
	result.ParsedOutputCount = len(result.ImageURLs)
	return result, nil
}

func (o providerBatchOrchestrator) fillProviderBatch(
	ctx context.Context,
	request providerRequest,
	requested int,
	maxCalls int,
	callCount *int,
	result *providerCallResult,
	call func(providerRequest) (providerCallResult, error),
) (lastErr error, err error) {
	fillAttempts := 0
	for len(result.ImageURLs) < requested && *callCount < maxCalls && fillAttempts < requested {
		fillAttempts++
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		single := request.withOutputCount(1)
		fillResult, fillErr := call(single)
		if fillErr == nil {
			mergeProviderBatchResult(result, fillResult, requested)
			continue
		}
		lastErr = fillErr
		if !shouldFailoverProviderError(fillErr) || *callCount >= maxCalls {
			break
		}
		if waitErr := o.wait(ctx, providerRetryDelay(fillErr)); waitErr != nil {
			return nil, waitErr
		}
		fillResult, fillErr = call(single)
		if fillErr == nil {
			mergeProviderBatchResult(result, fillResult, requested)
			// This retry belongs to the same missing slot. The outer loop only
			// advances after this slot has either produced an image or failed.
			continue
		}
		lastErr = fillErr
		// A slot gets at most one transient retry. Move on so one bad slot
		// cannot consume the retry budget intended for later outputs.
	}
	return lastErr, nil
}

func mergeProviderBatchResult(target *providerCallResult, current providerCallResult, limit int) {
	if target.Model == "" {
		target.Model = current.Model
	}
	if target.ParsedOutputKind == "" {
		target.ParsedOutputKind = current.ParsedOutputKind
	}
	if current.RawOutputPreview != "" {
		target.RawOutputPreview = current.RawOutputPreview
	}
	for _, imageURL := range normalizeProviderResultImageURLs(current) {
		if len(target.ImageURLs) >= limit {
			break
		}
		target.ImageURLs = append(target.ImageURLs, imageURL)
	}
}

func waitProviderRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func providerRetryDelay(err error) time.Duration {
	if retryAfter, ok := providerRetryAfter(err); ok {
		return retryAfter
	}
	const base = 250 * time.Millisecond
	var sample [8]byte
	if _, readErr := cryptorand.Read(sample[:]); readErr != nil {
		return base / 2
	}
	return base/2 + time.Duration(binary.BigEndian.Uint64(sample[:])%uint64(base/2+1))
}

func providerRetryAfter(err error) (time.Duration, bool) {
	var upstreamErr *upstreamHTTPError
	if !errors.As(err, &upstreamErr) || upstreamErr.RetryAfter <= 0 {
		return 0, false
	}
	return upstreamErr.RetryAfter, true
}

package worker

import (
	"context"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/tasking/tasks"
)

// providerFinalizer is the seam between generic provider execution and
// workflow-specific result handling. A finalizer may replace the upstream
// references and emit durable TaskOutput facts; returning stored=false leaves
// the references untouched so the pipeline can continue to the next step.
type providerFinalizer interface {
	finalize(
		ctx context.Context,
		userID int64,
		taskID string,
		imageRefs []string,
		timeout time.Duration,
	) (imageURLs []string, outputs []tasks.TaskOutput, stored bool, err error)
}

type providerFinalizationPipeline struct {
	featureFinalizers []providerFinalizer
	remoteFinalizer   providerFinalizer
}

func (p providerFinalizationPipeline) finalize(
	ctx context.Context,
	userID int64,
	taskID string,
	imageRefs []string,
	timeout time.Duration,
) ([]string, []tasks.TaskOutput, error) {
	currentRefs := imageRefs
	var outputs []tasks.TaskOutput
	stored := false
	for _, finalizer := range p.featureFinalizers {
		if finalizer == nil {
			continue
		}
		imageURLs, finalizerOutputs, didStore, err := finalizer.finalize(ctx, userID, taskID, currentRefs, timeout)
		if err != nil {
			return nil, nil, err
		}
		if !didStore {
			continue
		}
		currentRefs = imageURLs
		outputs = finalizerOutputs
		stored = true
	}
	if stored {
		return currentRefs, outputs, nil
	}
	if p.remoteFinalizer == nil {
		return currentRefs, nil, nil
	}
	imageURLs, remoteOutputs, _, err := p.remoteFinalizer.finalize(ctx, userID, taskID, currentRefs, timeout)
	if err != nil {
		return nil, nil, err
	}
	return imageURLs, remoteOutputs, nil
}

type localRedrawFinalizer struct {
	executor *ProviderExecutor
	context  *localRedrawContext
}

func (f localRedrawFinalizer) finalize(
	ctx context.Context,
	userID int64,
	taskID string,
	imageRefs []string,
	timeout time.Duration,
) ([]string, []tasks.TaskOutput, bool, error) {
	return f.executor.finalizeLocalRedrawResults(ctx, userID, taskID, f.context, imageRefs, timeout)
}

type imageExtendFinalizer struct {
	executor *ProviderExecutor
	context  *imageExtendContext
}

func (f imageExtendFinalizer) finalize(
	ctx context.Context,
	userID int64,
	taskID string,
	imageRefs []string,
	timeout time.Duration,
) ([]string, []tasks.TaskOutput, bool, error) {
	return f.executor.finalizeImageExtendResults(ctx, userID, taskID, f.context, imageRefs, timeout)
}

type remoteResultFinalizer struct {
	executor      *ProviderExecutor
	correlationID string
	account       *accounts.ApiAccount
}

func (f remoteResultFinalizer) finalize(
	ctx context.Context,
	userID int64,
	taskID string,
	imageRefs []string,
	_ time.Duration,
) ([]string, []tasks.TaskOutput, bool, error) {
	imageURLs, outputs, err := f.executor.materializeProviderResultImages(
		ctx,
		userID,
		taskID,
		f.correlationID,
		f.account,
		imageRefs,
	)
	return imageURLs, outputs, true, err
}

func (c *ProviderExecutor) prepareProviderFinalization(
	ctx context.Context,
	account *accounts.ApiAccount,
	params map[string]interface{},
	timeout time.Duration,
	correlationID string,
) (providerFinalizationPipeline, error) {
	localRedraw, err := c.prepareLocalRedrawPipeline(ctx, account, params, timeout)
	if err != nil {
		return providerFinalizationPipeline{}, err
	}
	imageExtend, err := c.prepareImageExtendPipeline(ctx, account, params, timeout)
	if err != nil {
		return providerFinalizationPipeline{}, err
	}

	featureFinalizers := make([]providerFinalizer, 0, 2)
	if localRedraw != nil {
		featureFinalizers = append(featureFinalizers, localRedrawFinalizer{executor: c, context: localRedraw})
	}
	if imageExtend != nil {
		featureFinalizers = append(featureFinalizers, imageExtendFinalizer{executor: c, context: imageExtend})
	}
	return providerFinalizationPipeline{
		featureFinalizers: featureFinalizers,
		remoteFinalizer: remoteResultFinalizer{
			executor:      c,
			correlationID: correlationID,
			account:       account,
		},
	}, nil
}

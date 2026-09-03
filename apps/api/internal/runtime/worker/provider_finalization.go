package worker

import (
	"context"
	"fmt"
	"strings"

	"ideaboard/api/internal/accountpool/accounts"
	tasks "ideaboard/api/internal/tasking/domain"
)

type providerResultFinalizationState uint8

const (
	// RemoteRefs is the only non-durable state. The universal materializer owns
	// its transition to DurableOutputs.
	providerResultRemoteRefs providerResultFinalizationState = iota + 1
	providerResultDurableOutputs
	providerResultTerminalFailure
)

// providerResultFinalization is a closed result protocol. A result belongs to
// exactly one state, so raw provider references cannot accidentally be treated
// as locally stored outputs or sent through a materializer twice.
type providerResultFinalization struct {
	state       providerResultFinalizationState
	remoteRefs  []string
	imageURLs   []string
	outputs     []tasks.TaskOutput
	terminalErr error
}

func newProviderRemoteRefs(imageRefs []string) providerResultFinalization {
	result := providerResultFinalization{
		state:      providerResultRemoteRefs,
		remoteRefs: append([]string(nil), imageRefs...),
	}
	return result.validatedOrTerminal()
}

func newProviderDurableOutputs(imageURLs []string, outputs []tasks.TaskOutput) providerResultFinalization {
	result := providerResultFinalization{
		state:     providerResultDurableOutputs,
		imageURLs: append([]string(nil), imageURLs...),
		outputs:   append([]tasks.TaskOutput(nil), outputs...),
	}
	return result.validatedOrTerminal()
}

func newProviderTerminalFailure(err error) providerResultFinalization {
	if err == nil {
		err = fmt.Errorf("provider result finalization failed without an error")
	}
	return providerResultFinalization{
		state:       providerResultTerminalFailure,
		terminalErr: err,
	}
}

func (r providerResultFinalization) validatedOrTerminal() providerResultFinalization {
	if err := r.validate(); err != nil {
		return newProviderTerminalFailure(err)
	}
	return r
}

func (r providerResultFinalization) validate() error {
	switch r.state {
	case providerResultRemoteRefs:
		return r.validateRemoteRefs()
	case providerResultDurableOutputs:
		return r.validateDurableOutputs()
	case providerResultTerminalFailure:
		return r.validateTerminalFailure()
	default:
		return fmt.Errorf("provider result state %d is unsupported", r.state)
	}
}

func (r providerResultFinalization) validateRemoteRefs() error {
	if len(r.remoteRefs) == 0 {
		return fmt.Errorf("provider result remote references are empty")
	}
	if len(r.imageURLs) != 0 || len(r.outputs) != 0 || r.terminalErr != nil {
		return fmt.Errorf("provider result remote references contain incompatible payload")
	}
	for _, ref := range r.remoteRefs {
		if strings.TrimSpace(ref) == "" {
			return fmt.Errorf("provider result contains an empty remote reference")
		}
	}
	return nil
}

func (r providerResultFinalization) validateDurableOutputs() error {
	if len(r.remoteRefs) != 0 || r.terminalErr != nil {
		return fmt.Errorf("provider result durable outputs contain incompatible payload")
	}
	if len(r.imageURLs) == 0 || len(r.imageURLs) != len(r.outputs) {
		return fmt.Errorf("provider result durable outputs are incomplete")
	}
	seenResultIndexes := make(map[int]struct{}, len(r.outputs))
	for index, output := range r.outputs {
		if err := validateDurableOutput(r.imageURLs[index], output); err != nil {
			return err
		}
		if _, exists := seenResultIndexes[output.ResultIndex]; exists {
			return fmt.Errorf("provider result writes result index %d more than once", output.ResultIndex)
		}
		seenResultIndexes[output.ResultIndex] = struct{}{}
	}
	return nil
}

func validateDurableOutput(imageURL string, output tasks.TaskOutput) error {
	if strings.TrimSpace(imageURL) == "" || strings.TrimSpace(output.URL) == "" {
		return fmt.Errorf("provider result durable output is missing an image URL")
	}
	if imageURL != output.URL {
		return fmt.Errorf("provider result durable output URL does not match image URL")
	}
	if output.ResultIndex < 0 {
		return fmt.Errorf("provider result durable output has a negative result index")
	}
	return nil
}

func (r providerResultFinalization) validateTerminalFailure() error {
	if r.terminalErr == nil {
		return fmt.Errorf("provider result terminal failure is missing an error")
	}
	if len(r.remoteRefs) != 0 || len(r.imageURLs) != 0 || len(r.outputs) != 0 {
		return fmt.Errorf("provider result terminal failure contains success payload")
	}
	return nil
}

func (r providerResultFinalization) completedOutputs() ([]string, []tasks.TaskOutput, error) {
	if err := r.validate(); err != nil {
		return nil, nil, err
	}
	if r.state == providerResultTerminalFailure {
		return nil, nil, r.terminalErr
	}
	if r.state != providerResultDurableOutputs {
		return nil, nil, fmt.Errorf("provider result finalization stopped before materialization")
	}
	return append([]string(nil), r.imageURLs...), append([]tasks.TaskOutput(nil), r.outputs...), nil
}

type providerResultFinalizer struct {
	executor      *ProviderExecutor
	correlationID string
	account       *accounts.ApiAccount
}

func (f providerResultFinalizer) finalize(
	ctx context.Context,
	userID int64,
	taskID string,
	imageRefs []string,
) providerResultFinalization {
	result := newProviderRemoteRefs(imageRefs)
	if result.state == providerResultTerminalFailure {
		return result
	}
	if f.executor == nil {
		return newProviderTerminalFailure(fmt.Errorf("provider executor is not initialized"))
	}

	return f.materializeRemoteRefs(ctx, userID, taskID, result)
}

func (f providerResultFinalizer) materializeRemoteRefs(ctx context.Context, userID int64, taskID string, result providerResultFinalization) providerResultFinalization {
	if err := result.validate(); err != nil {
		return newProviderTerminalFailure(err)
	}
	if result.state != providerResultRemoteRefs {
		return newProviderTerminalFailure(fmt.Errorf("remote materializer requires remote references"))
	}
	if f.executor == nil {
		return newProviderTerminalFailure(fmt.Errorf("provider executor is not initialized"))
	}
	imageURLs, outputs, err := f.executor.materializeProviderResultImages(
		ctx,
		userID,
		taskID,
		f.correlationID,
		f.account,
		result.remoteRefs,
	)
	if err != nil {
		return newProviderTerminalFailure(err)
	}
	return newProviderDurableOutputs(imageURLs, outputs)
}

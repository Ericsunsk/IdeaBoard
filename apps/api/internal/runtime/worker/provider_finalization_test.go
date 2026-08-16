package worker

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"ideaboard/api/internal/tasking/taskmedia"
	"ideaboard/api/internal/tasking/tasks"
)

func TestProviderResultFinalizerMaterializesRemoteRefsOnce(t *testing.T) {
	materializer := &captureResultMaterializer{
		remoteResult: taskmedia.MaterializedResults{Items: []taskmedia.MaterializedResult{{
			ImageURL: "/uploads/tasks/42/task-1/result.png",
			Output: tasks.TaskOutput{
				TaskID: "task-1", URL: "/uploads/tasks/42/task-1/result.png", ResultIndex: 0,
			},
		}}},
	}
	finalizer := providerResultFinalizer{
		executor:    &ProviderExecutor{resultMaterializer: materializer},
		requirement: providerFinalizationRemoteResult,
	}

	result := finalizer.finalize(context.Background(), 42, "task-1", []string{"https://provider.example/result.png"}, 0)
	if result.state != providerResultDurableOutputs {
		t.Fatalf("state=%d, want durable outputs", result.state)
	}
	urls, outputs, err := result.completedOutputs()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(urls, []string{"/uploads/tasks/42/task-1/result.png"}) || len(outputs) != 1 {
		t.Fatalf("urls/outputs=%#v/%#v", urls, outputs)
	}
	if !reflect.DeepEqual(materializer.remoteSourceURLs, []string{"https://provider.example/result.png"}) {
		t.Fatalf("remote materializer sources=%#v", materializer.remoteSourceURLs)
	}
	if materializer.remoteCalls != 1 {
		t.Fatalf("remote materializer calls=%d, want 1", materializer.remoteCalls)
	}
}

func TestProviderResultFinalizerTurnsStorageErrorIntoTerminalFailure(t *testing.T) {
	storeErr := errors.New("materializer unavailable")
	materializer := &captureResultMaterializer{remoteErr: storeErr}
	finalizer := providerResultFinalizer{
		executor:    &ProviderExecutor{resultMaterializer: materializer},
		requirement: providerFinalizationRemoteResult,
	}

	result := finalizer.finalize(context.Background(), 42, "task-1", []string{"https://provider.example/result.png"}, 0)
	if result.state != providerResultTerminalFailure {
		t.Fatalf("state=%d, want terminal failure", result.state)
	}
	if _, _, err := result.completedOutputs(); !errors.Is(err, storeErr) {
		t.Fatalf("err=%v, want wrapped storage error", err)
	}
	if materializer.remoteCalls != 1 {
		t.Fatalf("remote materializer calls=%d, want 1", materializer.remoteCalls)
	}
}

func TestProviderResultFinalizationRejectsDuplicateResultSlots(t *testing.T) {
	result := newProviderDurableOutputs(
		[]string{"/uploads/first.png", "/uploads/second.png"},
		[]tasks.TaskOutput{
			{URL: "/uploads/first.png", ResultIndex: 1},
			{URL: "/uploads/second.png", ResultIndex: 1},
		},
	)
	if result.state != providerResultTerminalFailure {
		t.Fatalf("state=%d, want terminal failure", result.state)
	}
	if _, _, err := result.completedOutputs(); err == nil || !strings.Contains(err.Error(), "more than once") {
		t.Fatalf("err=%v, want duplicate slot failure", err)
	}
}

func TestProviderResultFinalizationRequiresMaterializedOutputs(t *testing.T) {
	result := newProviderRemoteRefs([]string{"https://provider.example/result.png"})
	if _, _, err := result.completedOutputs(); err == nil || !strings.Contains(err.Error(), "before materialization") {
		t.Fatalf("err=%v, want incomplete state failure", err)
	}
}

func TestProviderResultFinalizationRejectsMismatchedOutputURL(t *testing.T) {
	result := newProviderAlreadyMaterialized(
		[]string{"/uploads/result.png"},
		[]tasks.TaskOutput{{URL: "/uploads/other.png", ResultIndex: 0}},
	)
	if result.state != providerResultTerminalFailure {
		t.Fatalf("state=%d, want terminal failure", result.state)
	}
	if _, _, err := result.completedOutputs(); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("err=%v, want mismatched URL failure", err)
	}
}

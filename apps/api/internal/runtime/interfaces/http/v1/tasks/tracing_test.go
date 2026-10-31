package tasks

import (
	"context"
	"reflect"
	"testing"

	taskdomain "ideaboard/api/internal/tasking/domain"
)

type recordingSettlementTracer struct {
	names []string
	ended int
}

func (t *recordingSettlementTracer) Start(ctx context.Context, name string) (context.Context, func(error)) {
	t.names = append(t.names, name)
	return ctx, func(error) { t.ended++ }
}

func TestTaskServicePropagatesSettlementTracer(t *testing.T) {
	tracer := &recordingSettlementTracer{}
	h := NewHandler(Deps{Tasks: ContractV1TaskDeps{TaskTracer: tracer}})
	service := h.taskService()
	ctx := t.Context()
	// Missing stores fail closed, but every attempted settlement is traced.
	_, _ = service.Create(ctx, taskdomain.CreateInput{}, "submit")
	_, _, _ = service.Cancel(ctx, "ws_test", "task_test", 1, "")
	_, _ = service.Complete(ctx, "ws_test", "task_test", nil)
	_, _ = service.Fail(ctx, "ws_test", "task_test", "failed")
	_, _ = service.Retry(ctx, "ws_test", "task_test", true)
	want := []string{
		"generation.submit.settlement",
		"generation.terminal.settlement.cancel",
		"generation.terminal.settlement.complete",
		"generation.terminal.settlement.fail",
		"generation.terminal.settlement.retry",
	}
	if !reflect.DeepEqual(tracer.names, want) || tracer.ended != len(want) {
		t.Fatalf("settlement spans = %v, ended = %d; want %v, all ended", tracer.names, tracer.ended, want)
	}
}

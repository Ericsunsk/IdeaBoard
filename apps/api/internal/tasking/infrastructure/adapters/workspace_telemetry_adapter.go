package adapters

import (
	"context"

	"ideaboard/api/internal/platform/telemetry"
	taskports "ideaboard/api/internal/tasking/ports/tasks"
)

type workspaceSettlementTracer struct{}

// NewWorkspaceSettlementTracer creates a SettlementTracer backed by platform/telemetry.
func NewWorkspaceSettlementTracer() taskports.SettlementTracer {
	return &workspaceSettlementTracer{}
}

func (t *workspaceSettlementTracer) Start(ctx context.Context, name string) (context.Context, func(err error)) {
	ctx, span := telemetry.Start(ctx, name)
	return ctx, func(err error) {
		telemetry.End(span, err)
	}
}

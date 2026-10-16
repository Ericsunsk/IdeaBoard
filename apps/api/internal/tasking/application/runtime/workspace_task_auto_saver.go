package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	librarydomain "ideaboard/api/internal/library/domain"
	"ideaboard/api/internal/tasking/workspacetasks"
)

// TaskOutputSaveInput identifies settled media and its Task provenance.
type TaskOutputSaveInput struct {
	TaskID         string
	ResultIndex    int
	MediaAssetID   string
	OperatorUserID int64
}

// TaskOutputLibrarySaver saves a managed reference in Quick Saves,
// deduplicated by Workspace and Media identity.
type TaskOutputLibrarySaver interface {
	SaveTaskOutputAsset(ctx context.Context, workspacePublicID string, input TaskOutputSaveInput) (*librarydomain.V1Asset, bool, error)
}

// WorkspaceTaskAutoSaver saves settled outputs after Task completion.
// Per-output failures never change the completed Task or requeue execution.
type WorkspaceTaskAutoSaver interface {
	SaveCompletedTask(ctx context.Context, task *workspacetasks.WorkspaceTask) error
}

type workspaceTaskAutoSaver struct {
	saver TaskOutputLibrarySaver
}

func NewWorkspaceTaskAutoSaver(saver TaskOutputLibrarySaver) (WorkspaceTaskAutoSaver, error) {
	if saver == nil {
		return nil, fmt.Errorf("task auto-saver: library saver is required")
	}
	return &workspaceTaskAutoSaver{saver: saver}, nil
}

func (s *workspaceTaskAutoSaver) SaveCompletedTask(ctx context.Context, task *workspacetasks.WorkspaceTask) error {
	if s == nil || s.saver == nil {
		return fmt.Errorf("task auto-saver is not configured")
	}
	if task == nil || strings.TrimSpace(task.ID) == "" || strings.TrimSpace(task.WorkspaceID) == "" {
		return fmt.Errorf("task auto-saver: task identity required")
	}
	for _, ref := range task.OutputRefs {
		if ref.Kind != workspacetasks.OutputKindAsset || strings.TrimSpace(ref.AssetID) == "" {
			continue
		}
		if saved, _, err := s.saver.SaveTaskOutputAsset(ctx, strings.TrimSpace(task.WorkspaceID), TaskOutputSaveInput{
			TaskID: task.ID, ResultIndex: ref.ResultIndex,
			MediaAssetID: strings.TrimSpace(ref.AssetID), OperatorUserID: task.OwnerUserID,
		}); err != nil || saved == nil {
			slog.Warn("task auto-save: Quick Saves save failed",
				"task_id", task.ID, "result_index", ref.ResultIndex, "err", err)
		}
	}
	return nil
}

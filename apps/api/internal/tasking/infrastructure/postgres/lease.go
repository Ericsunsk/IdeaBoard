package postgres

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"ideaboard/api/internal/db/dbutil"

	dbsqlc "ideaboard/api/internal/db/sqlc"
	tasks "ideaboard/api/internal/tasking/domain"

	"github.com/jackc/pgx/v5/pgtype"
)

func insertTaskOutputs(ctx context.Context, q *dbsqlc.Queries, taskID string, ownerUserID int64, outputs []tasks.TaskOutput) error {
	lineageRefs, err := q.ListTaskInputMediaRefsForLineage(ctx, taskID)
	if err != nil {
		return fmt.Errorf("load task input lineage: %w", err)
	}
	for i := range outputs {
		output := outputs[i]
		output.TaskID = taskID
		output.OutputKind = normalizeTaskOutputKind(output)
		if strings.TrimSpace(output.Role) == "" {
			output.Role = "image"
		}
		if output.OutputKind == "asset" {
			mediaAssetID, err := resolveTaskOutputMediaAsset(ctx, q, ownerUserID, output)
			if err != nil {
				return err
			}
			output.MediaAssetID = mediaAssetID
			output.URL = ""
		} else {
			output.MediaAssetID = nil
		}
		taskOutputID, err := q.CreateTaskOutput(ctx, dbsqlc.CreateTaskOutputParams{
			TaskID:       output.TaskID,
			MediaAssetID: dbutil.NullInt64(output.MediaAssetID),
			OutputKind:   output.OutputKind,
			Role:         output.Role,
			Url:          output.URL,
			SourceUrl:    output.SourceURL,
			Metadata:     outputMetadataJSON(output.Metadata),
			ResultIndex:  int32(output.ResultIndex),
		})
		if err != nil {
			return err
		}
		if output.MediaAssetID != nil {
			if _, err := q.UpsertMediaRef(ctx, dbsqlc.UpsertMediaRefParams{
				MediaAssetID: *output.MediaAssetID,
				OwnerUserID:  ownerUserID,
				RefType:      "task_output",
				RefID:        strconv.FormatInt(taskOutputID, 10),
				BoardID:      "",
				TaskID:       taskID,
				Role:         output.Role,
				Metadata:     mediaRefMetadataJSON(taskOutputID),
			}); err != nil {
				return err
			}
			for _, source := range lineageRefs {
				if source.MediaAssetID == *output.MediaAssetID {
					continue
				}
				sourceRole := strings.TrimSpace(source.Role)
				if sourceRole == "" {
					sourceRole = "source"
				}
				if _, err := q.UpsertMediaAssetSource(ctx, dbsqlc.UpsertMediaAssetSourceParams{
					MediaAssetID:  *output.MediaAssetID,
					SourceAssetID: source.MediaAssetID,
					OwnerUserID:   ownerUserID,
					SourceRole:    sourceRole,
					SourceIndex:   source.InputIndex,
					Metadata:      []byte(`{"origin":"task_input"}`),
				}); err != nil {
					return fmt.Errorf("persist task output lineage: %w", err)
				}
			}
		}
	}
	return nil
}

func normalizeTaskOutputKind(output tasks.TaskOutput) string {
	switch strings.TrimSpace(output.OutputKind) {
	case "remote":
		return "remote"
	default:
		return "asset"
	}
}

func resolveTaskOutputMediaAsset(ctx context.Context, q *dbsqlc.Queries, ownerUserID int64, output tasks.TaskOutput) (*int64, error) {
	if output.MediaAssetID != nil && *output.MediaAssetID > 0 {
		asset, err := q.GetMediaAssetByIDForOwner(ctx, dbsqlc.GetMediaAssetByIDForOwnerParams{
			ID: *output.MediaAssetID, OwnerUserID: ownerUserID,
		})
		if err != nil {
			return nil, fmt.Errorf("load task result media asset %d: %w", *output.MediaAssetID, err)
		}
		if asset.Status != "ready" || asset.DeletedAt.Valid {
			return nil, fmt.Errorf("task result media asset %d is not ready", *output.MediaAssetID)
		}
		if storageKey := strings.TrimSpace(output.StorageKey); storageKey != "" && storageKey != asset.StorageKey {
			return nil, fmt.Errorf("task result media asset %d storage key does not match", *output.MediaAssetID)
		}
		return output.MediaAssetID, nil
	}
	return upsertTaskOutputMediaAsset(ctx, q, ownerUserID, output)
}

func upsertTaskOutputMediaAsset(ctx context.Context, q *dbsqlc.Queries, ownerUserID int64, output tasks.TaskOutput) (*int64, error) {
	storageKey := strings.TrimSpace(output.StorageKey)
	if ownerUserID == 0 || storageKey == "" {
		return nil, nil
	}
	assetID, err := newMediaAssetID()
	if err != nil {
		return nil, err
	}
	storageProfileID := strings.TrimSpace(output.StorageProfile)
	if storageProfileID == "" {
		storageProfileID = defaultStorageProfileID
	}
	id, err := q.UpsertMediaAsset(ctx, dbsqlc.UpsertMediaAssetParams{
		AssetID:          assetID,
		OwnerUserID:      ownerUserID,
		StorageProfileID: storageProfileID,
		StorageKey:       storageKey,
		Url:              strings.TrimSpace(output.URL),
		SourceUrl:        strings.TrimSpace(output.SourceURL),
		MediaKind:        "image",
		MimeType:         strings.TrimSpace(output.MIMEType),
		Width:            int32(output.Width),
		Height:           int32(output.Height),
		SizeBytes:        output.SizeBytes,
		ChecksumSha256:   strings.TrimSpace(output.ChecksumSHA256),
		Status:           "ready",
		Metadata:         mediaAssetMetadataJSON(output),
	})
	if err != nil {
		return nil, err
	}
	// Display tiers resolve on demand through imgproxy; the provider-neutral
	// processing-input derivative materializes on demand at intake validation
	// or manifest build time.
	return &id, nil
}

func (r *Repository) MarkTaskCancelled(ctx context.Context, snapshot tasks.TaskCancellationSnapshot, attemptFinish *tasks.TaskAttemptFinish) (int64, error) {
	taskID := strings.TrimSpace(snapshot.TaskID)
	if taskID == "" {
		return 0, fmt.Errorf("task_id is required")
	}
	cancelledAt := terminalTransitionTime(snapshot.CancelledAt)
	reason := strings.TrimSpace(snapshot.Reason)
	return r.runTerminalTransition(ctx, terminalTransitionCommand{
		taskID:        taskID,
		kind:          terminalTransitionCancelled,
		finishedAt:    cancelledAt,
		errorMessage:  reason,
		attemptFinish: attemptFinish,
		writeTaskFact: func(ctx context.Context, q *dbsqlc.Queries) (int64, error) {
			return q.MarkTaskCancelled(ctx, dbsqlc.MarkTaskCancelledParams{
				CancelledAt:  pgtype.Timestamptz{Time: cancelledAt, Valid: true},
				CancelledBy:  dbutil.NullInt64(snapshot.CancelledBy),
				CancelReason: reason,
				TaskID:       taskID,
			})
		},
	})
}

func (r *Repository) ListRunningTasksForStartupRecovery(ctx context.Context) ([]StartupRecoveredTask, error) {
	rows, err := r.q.ListRunningTasksForStartupRecovery(ctx)
	if err != nil {
		return nil, err
	}
	recoveredTasks := make([]StartupRecoveredTask, 0, len(rows))
	for _, task := range rows {
		recoveredTasks = append(recoveredTasks, StartupRecoveredTask{
			TaskID:     task.TaskID,
			RiverJobID: task.RiverJobID,
		})
	}
	return recoveredTasks, nil
}

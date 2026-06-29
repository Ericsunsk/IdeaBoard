package assets

import (
	"context"
	"database/sql"

	dbsqlc "ideaboard/api/internal/db/sqlc"
)

type LineDraftModeRepository struct {
	q *dbsqlc.Queries
}

func NewLineDraftModeRepository(db *sql.DB) *LineDraftModeRepository {
	return &LineDraftModeRepository{q: dbsqlc.New(db)}
}

func (r *LineDraftModeRepository) Create(ctx context.Context, mode *LineDraftMode) error {
	id, err := r.q.CreateLineDraftMode(ctx, dbsqlc.CreateLineDraftModeParams{
		Name:       mode.Name,
		CoverImage: mode.CoverImage,
		Prompt:     mode.Prompt,
		SortOrder:  int32(mode.SortOrder),
		IsEnabled:  mode.IsEnabled,
	})
	if err != nil {
		return err
	}
	mode.ID = int64(id)
	return nil
}

func (r *LineDraftModeRepository) GetByID(ctx context.Context, id int64) (*LineDraftMode, error) {
	row, err := r.q.GetLineDraftModeByID(ctx, id)
	if err != nil {
		return nil, err
	}
	mode := lineDraftModeFromSQLC(row)
	return &mode, nil
}

func (r *LineDraftModeRepository) List(ctx context.Context) ([]*LineDraftMode, error) {
	rows, err := r.q.ListLineDraftModes(ctx)
	if err != nil {
		return nil, err
	}
	return lineDraftModesFromSQLC(rows), nil
}

func (r *LineDraftModeRepository) ListEnabled(ctx context.Context) ([]*LineDraftMode, error) {
	rows, err := r.q.ListEnabledLineDraftModes(ctx)
	if err != nil {
		return nil, err
	}
	return lineDraftModesFromSQLC(rows), nil
}

func (r *LineDraftModeRepository) Update(ctx context.Context, mode *LineDraftMode) error {
	return r.q.UpdateLineDraftMode(ctx, dbsqlc.UpdateLineDraftModeParams{
		Name:       mode.Name,
		CoverImage: mode.CoverImage,
		Prompt:     mode.Prompt,
		SortOrder:  int32(mode.SortOrder),
		IsEnabled:  mode.IsEnabled,
		ID:         mode.ID,
	})
}

func (r *LineDraftModeRepository) Delete(ctx context.Context, id int64) error {
	return r.q.DeleteLineDraftMode(ctx, id)
}

func lineDraftModeFromSQLC(row dbsqlc.LineDraftMode) LineDraftMode {
	return LineDraftMode{
		ID:         row.ID,
		Name:       row.Name,
		CoverImage: row.CoverImage,
		Prompt:     row.Prompt,
		SortOrder:  int(row.SortOrder),
		IsEnabled:  row.IsEnabled,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
}

func lineDraftModesFromSQLC(rows []dbsqlc.LineDraftMode) []*LineDraftMode {
	result := make([]*LineDraftMode, 0, len(rows))
	for _, row := range rows {
		mode := lineDraftModeFromSQLC(row)
		result = append(result, &mode)
	}
	return result
}

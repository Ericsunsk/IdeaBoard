package channels

import (
	"context"
	"database/sql"

	dbsqlc "ideaboard/api/internal/db/sqlc"
)

type ChannelRepository struct {
	q *dbsqlc.Queries
}

func NewChannelRepository(db *sql.DB) *ChannelRepository {
	return &ChannelRepository{q: dbsqlc.New(db)}
}

func (r *ChannelRepository) Create(ctx context.Context, channel *Channel) error {
	id, err := r.q.CreateChannel(ctx, dbsqlc.CreateChannelParams{
		Type:   channel.Type,
		Name:   channel.Name,
		ApiUrl: channel.ApiURL,
		Status: channel.Status,
	})
	if err != nil {
		return err
	}
	channel.ID = int64(id)
	return nil
}

func (r *ChannelRepository) GetByID(ctx context.Context, id int64) (*Channel, error) {
	row, err := r.q.GetChannelByID(ctx, id)
	if err != nil {
		return nil, err
	}
	channel := Channel{
		ID:        row.ID,
		Type:      row.Type,
		Name:      row.Name,
		ApiURL:    row.ApiUrl,
		Status:    row.Status,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
	return &channel, nil
}

func (r *ChannelRepository) List(ctx context.Context) ([]*Channel, error) {
	rows, err := r.q.ListChannels(ctx)
	if err != nil {
		return nil, err
	}
	channels := make([]*Channel, 0, len(rows))
	for _, row := range rows {
		channel := Channel{
			ID:        row.ID,
			Type:      row.Type,
			Name:      row.Name,
			ApiURL:    row.ApiUrl,
			Status:    row.Status,
			CreatedAt: row.CreatedAt,
			UpdatedAt: row.UpdatedAt,
		}
		channels = append(channels, &channel)
	}
	return channels, nil
}

func (r *ChannelRepository) Update(ctx context.Context, channel *Channel) error {
	return r.q.UpdateChannel(ctx, dbsqlc.UpdateChannelParams{
		Type:   channel.Type,
		Name:   channel.Name,
		ApiUrl: channel.ApiURL,
		Status: channel.Status,
		ID:     channel.ID,
	})
}

func (r *ChannelRepository) Delete(ctx context.Context, id int64) error {
	return r.q.DeleteChannel(ctx, id)
}

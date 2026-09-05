package database

import (
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pools holds runtime-owned database pool references.
type Pools struct {
	DB        *sql.DB
	RiverPool *pgxpool.Pool
}

// Close gracefully releases both PostgreSQL and River pools.
func (p *Pools) Close() error {
	if p == nil {
		return nil
	}
	var errs []error
	if p.RiverPool != nil {
		p.RiverPool.Close()
	}
	if p.DB != nil {
		if err := p.DB.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

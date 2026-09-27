package rewardcodes

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

type recordingPool struct {
	beginCalls   int
	beginTxCalls int
	options      pgx.TxOptions
}

func (p *recordingPool) Begin(context.Context) (pgx.Tx, error) {
	p.beginCalls++
	return nil, nil
}

func (p *recordingPool) BeginTx(_ context.Context, options pgx.TxOptions) (pgx.Tx, error) {
	p.beginTxCalls++
	p.options = options
	return nil, nil
}

func (p *recordingPool) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, nil
}

func (p *recordingPool) QueryRow(context.Context, string, ...any) pgx.Row {
	return nil
}

func TestBeginReadSnapshotUsesRepeatableReadReadOnlyTransaction(t *testing.T) {
	pool := &recordingPool{}
	service := &Service{pool: pool}

	if _, err := service.beginReadSnapshot(context.Background()); err != nil {
		t.Fatalf("beginReadSnapshot() error = %v", err)
	}
	if pool.beginTxCalls != 1 || pool.beginCalls != 0 {
		t.Fatalf("transaction calls = BeginTx:%d Begin:%d, want BeginTx:1 Begin:0", pool.beginTxCalls, pool.beginCalls)
	}
	if pool.options.IsoLevel != pgx.RepeatableRead || pool.options.AccessMode != pgx.ReadOnly {
		t.Fatalf("transaction options = %+v, want repeatable-read read-only", pool.options)
	}
}

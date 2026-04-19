package store

import (
	"context"
	"errors"

	"github.com/abartrim/sobs/internal/extensionpoints"
)

type NoopStoreFactory struct{}

type NoopStore struct{}

type NoopRows struct{}

func (r *NoopRows) Columns() ([]string, error) {
	return []string{}, nil
}

type NoopResult struct{}

func NewNoopStoreFactory() extensionpoints.StoreFactory {
	return &NoopStoreFactory{}
}

func (f *NoopStoreFactory) Open(ctx context.Context) (extensionpoints.ClickHouseStore, error) {
	_ = ctx
	return &NoopStore{}, nil
}

func (s *NoopStore) Ping(ctx context.Context) error {
	_ = ctx
	return nil
}

func (s *NoopStore) Query(ctx context.Context, query string, args ...any) (extensionpoints.RowIterator, error) {
	_ = ctx
	_ = query
	_ = args
	return &NoopRows{}, nil
}

func (s *NoopStore) Exec(ctx context.Context, query string, args ...any) (extensionpoints.Result, error) {
	_ = ctx
	_ = query
	_ = args
	return &NoopResult{}, nil
}

func (s *NoopStore) Close() error {
	return nil
}

func (r *NoopRows) Next() bool {
	return false
}

func (r *NoopRows) Scan(dest ...any) error {
	_ = dest
	return errors.New("no rows")
}

func (r *NoopRows) Err() error {
	return nil
}

func (r *NoopRows) Close() error {
	return nil
}

func (r *NoopResult) RowsAffected() (int64, error) {
	return 0, nil
}

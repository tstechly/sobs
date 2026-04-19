package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/abartrim/sobs/internal/extensionpoints"
	_ "github.com/chdb-io/chdb-go/chdb/driver"
)

const (
	defaultChdbPath = "data/sobs.chdb"
)

type ChdbStoreFactory struct {
	path string
}

type ChdbStore struct {
	db *sql.DB
}

type SQLRows struct {
	rows *sql.Rows
}

func (r *SQLRows) Columns() ([]string, error) {
	return r.rows.Columns()
}

type SQLResult struct {
	result sql.Result
}

func NewChdbStoreFactory(path string) extensionpoints.StoreFactory {
	resolvedPath := path
	if resolvedPath == "" {
		resolvedPath = defaultChdbPath
	}
	return &ChdbStoreFactory{path: resolvedPath}
}

func NewChdbStoreFactoryFromEnv() extensionpoints.StoreFactory {
	return NewChdbStoreFactory(os.Getenv("SOBS_CHDB_PATH"))
}

func (f *ChdbStoreFactory) Open(ctx context.Context) (extensionpoints.ClickHouseStore, error) {
	_ = ctx
	absPath, err := filepath.Abs(f.path)
	if err != nil {
		return nil, fmt.Errorf("resolve chdb path: %w", err)
	}
	if err := os.MkdirAll(absPath, 0o755); err != nil {
		return nil, fmt.Errorf("create chdb path: %w", err)
	}

	connStr := fmt.Sprintf("session=%s;driverType=parquet", absPath)
	db, err := sql.Open("chdb", connStr)
	if err != nil {
		return nil, fmt.Errorf("open chdb: %w", err)
	}
	store := &ChdbStore{db: db}
	if err := store.Ping(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.ExecContext(
		context.Background(),
		"CREATE TABLE IF NOT EXISTS sobs_app_settings (" +
			"Key String," +
			"Value String," +
			"UpdatedAt DateTime64(3) DEFAULT now64(3)" +
			") ENGINE = ReplacingMergeTree(UpdatedAt) ORDER BY Key",
	); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ensure sobs_app_settings schema: %w", err)
	}
	return store, nil
}

func (s *ChdbStore) Ping(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.db.PingContext(ctx)
}

func (s *ChdbStore) Query(ctx context.Context, query string, args ...any) (extensionpoints.RowIterator, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &SQLRows{rows: rows}, nil
}

func (s *ChdbStore) Exec(ctx context.Context, query string, args ...any) (extensionpoints.Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &SQLResult{result: result}, nil
}

func (s *ChdbStore) Close() error {
	return s.db.Close()
}

func (r *SQLRows) Next() bool {
	return r.rows.Next()
}

func (r *SQLRows) Scan(dest ...any) error {
	return r.rows.Scan(dest...)
}

func (r *SQLRows) Err() error {
	return r.rows.Err()
}

func (r *SQLRows) Close() error {
	return r.rows.Close()
}

func (r *SQLResult) RowsAffected() (int64, error) {
	return r.result.RowsAffected()
}


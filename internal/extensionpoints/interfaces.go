package extensionpoints

import (
	"context"
	"net/http"
)

type Identity struct {
	Subject string
	Email   string
	Roles   []string
}

type AuthProvider interface {
	Authenticate(ctx context.Context, r *http.Request) (Identity, error)
	Authorize(ctx context.Context, id Identity, permission string) error
}

type RowIterator interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close() error
}

type Result interface {
	RowsAffected() (int64, error)
}

type ClickHouseStore interface {
	Ping(ctx context.Context) error
	Query(ctx context.Context, query string, args ...any) (RowIterator, error)
	Exec(ctx context.Context, query string, args ...any) (Result, error)
	Close() error
}

type StoreFactory interface {
	Open(ctx context.Context) (ClickHouseStore, error)
}

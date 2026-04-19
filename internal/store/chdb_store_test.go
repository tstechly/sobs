package store

import (
	"context"
	"testing"
)

func TestChdbStoreFactoryPersistsSettings(t *testing.T) {
	tmp := t.TempDir()
	factory := NewChdbStoreFactory(tmp)

	storeA, err := factory.Open(context.Background())
	if err != nil {
		t.Fatalf("open store A: %v", err)
	}
	if err := storeA.Ping(context.Background()); err != nil {
		t.Fatalf("ping store A: %v", err)
	}
	if _, err := storeA.Exec(context.Background(), "INSERT INTO sobs_app_settings (Key, Value) VALUES (?, ?)", "k", "v1"); err != nil {
		t.Fatalf("insert setting: %v", err)
	}
	_ = storeA.Close()

	storeB, err := factory.Open(context.Background())
	if err != nil {
		t.Fatalf("open store B: %v", err)
	}
	defer func() { _ = storeB.Close() }()
	rows, err := storeB.Query(context.Background(), "SELECT Value FROM sobs_app_settings WHERE Key = ? ORDER BY UpdatedAt DESC LIMIT 1", "k")
	if err != nil {
		t.Fatalf("query setting: %v", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatal("expected one row")
	}
	var value string
	if err := rows.Scan(&value); err != nil {
		t.Fatalf("scan value: %v", err)
	}
	if value != "v1" {
		t.Fatalf("expected value v1, got %q", value)
	}
}

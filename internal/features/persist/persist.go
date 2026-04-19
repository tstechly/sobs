package persist

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/google/uuid"
)

func NewID() string {
	return uuid.NewString()
}

func Version() uint64 {
	return uint64(time.Now().UnixNano())
}

func RFC3339Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func Open(ctx context.Context, factory extensionpoints.StoreFactory) (extensionpoints.ClickHouseStore, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return factory.Open(ctx)
}

func JSONString(value any) string {
	blob, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(blob)
}

func ParseJSONMap(raw string) map[string]any {
	out := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]any{}
	}
	return out
}

func ParseJSONStringSlice(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err == nil {
		return out
	}
	parts := strings.Split(raw, ",")
	out = make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func ParseStringMap(raw string) map[string]string {
	out := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]string{}
	}
	return out
}

func GetAppSetting(ctx context.Context, factory extensionpoints.StoreFactory, key string) (string, bool, error) {
	store, err := Open(ctx, factory)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT Value FROM sobs_app_settings WHERE Key = ? ORDER BY UpdatedAt DESC LIMIT 1", key)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return "", false, err
		}
		return "", false, nil
	}
	var value string
	if err := rows.Scan(&value); err != nil {
		return "", false, err
	}
	return value, true, nil
}

func SetAppSetting(ctx context.Context, factory extensionpoints.StoreFactory, key string, value string) error {
	store, err := Open(ctx, factory)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	_, err = store.Exec(ctx, "INSERT INTO sobs_app_settings (Key, Value) VALUES (?, ?)", key, value)
	return err
}

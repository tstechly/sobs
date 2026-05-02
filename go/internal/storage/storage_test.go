package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIsWritableTable(t *testing.T) {
	tests := []struct {
		name  string
		table string
		want  bool
	}{
		{name: "otlp logs", table: "otel_logs", want: true},
		{name: "pinned metrics", table: "otel_metrics_gauge_pinned", want: true},
		{name: "github work items", table: "sobs_github_work_items", want: true},
		{name: "raw windows", table: "sobs_raw_windows", want: true},
		{name: "unknown table", table: "not_a_table", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isWritableTable(tc.table))
		})
	}
}

func TestNormalizeJSONRows(t *testing.T) {
	ts := time.Date(2026, 5, 2, 3, 4, 5, 123456000, time.UTC)
	rows := []map[string]any{
		{
			"Timestamp": ts,
			"CreatedAt": "2026-05-02T03:04:05.123456Z",
			"Events": map[string]any{
				"Timestamp": "2026-05-02T03:04:05.123456Z",
			},
		},
	}

	got := normalizeJSONRows(rows)
	require.Len(t, got, 1)
	require.Equal(t, "2026-05-02 03:04:05.123456", got[0]["Timestamp"])
	require.Equal(t, "2026-05-02 03:04:05.123456", got[0]["CreatedAt"])
	events, ok := got[0]["Events"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "2026-05-02 03:04:05.123456", events["Timestamp"])
}

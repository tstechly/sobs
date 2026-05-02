package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	nethttp "net/http"
	neturl "net/url"
	"strings"
	"sync"
	"time"
)

// DB wraps a *sql.DB with schema bootstrap and a serialised write queue.
type DB struct {
	conn    *sql.DB
	httpURL string // ClickHouse HTTP API base URL for JSONEachRow inserts
	mu      sync.Mutex
	ready   bool
	writeCh chan writeTask
	done    chan struct{}
}

type writeTask struct {
	op   func(*DB) error
	errC chan error
}

// New creates a DB wrapper. It attempts to connect but starts the write
// worker regardless so the server can boot even when the database is
// temporarily unavailable.  The caller must call Close when finished.
// httpURL is the ClickHouse HTTP endpoint (e.g. "http://localhost:8123").
func New(driver, dsn, httpURL string, queueSize int) (*DB, error) {
	conn, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("storage open: %w", err)
	}
	d := &DB{
		conn:    conn,
		httpURL: httpURL,
		writeCh: make(chan writeTask, queueSize),
		done:    make(chan struct{}),
	}
	go d.writeWorker()
	// Best-effort initial ping — log but do not block startup.
	if err := conn.Ping(); err != nil {
		slog.Warn("database not reachable at startup (will retry on first use)", "error", err)
	}
	return d, nil
}

// Bootstrap applies the schema DDL. Each statement is executed individually
// because ClickHouse does not support multi-statement exec.
// Safe to call multiple times.
func (d *DB) Bootstrap(ctx context.Context, ddl string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.ready {
		return nil
	}
	stmts := splitStatements(ddl)
	for _, stmt := range stmts {
		if _, err := d.conn.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("schema bootstrap: %w", err)
		}
	}
	d.ready = true
	return nil
}

// splitStatements splits DDL text on semicolons, filtering blanks.
func splitStatements(ddl string) []string {
	raw := strings.Split(ddl, ";")
	var out []string
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s != "" && !strings.HasPrefix(s, "--") {
			out = append(out, s)
		}
	}
	return out
}

// Conn returns the underlying *sql.DB for direct reads.
func (d *DB) Conn() *sql.DB { return d.conn }

// Ping verifies the connection is alive.
func (d *DB) Ping(ctx context.Context) error { return d.conn.PingContext(ctx) }

// QueueWrite enqueues an operation on the serialised write worker.
func (d *DB) QueueWrite(op func(*DB) error) error {
	t := writeTask{op: op, errC: make(chan error, 1)}
	select {
	case d.writeCh <- t:
	default:
		return fmt.Errorf("write queue is full")
	}
	return <-t.errC
}

// QueueWriteAsync enqueues without waiting for completion.
func (d *DB) QueueWriteAsync(op func(*DB) error) error {
	t := writeTask{op: op, errC: make(chan error, 1)}
	select {
	case d.writeCh <- t:
		return nil
	default:
		return fmt.Errorf("write queue is full")
	}
}

// WriteQueueDepth returns current pending writes.
func (d *DB) WriteQueueDepth() int { return len(d.writeCh) }

func (d *DB) writeWorker() {
	for t := range d.writeCh {
		err := t.op(d)
		if err != nil {
			slog.Error("write worker op failed", "error", err)
		}
		select {
		case t.errC <- err:
		default:
		}
	}
	close(d.done)
}

// Close shuts down the write worker and closes the connection.
func (d *DB) Close() error {
	close(d.writeCh)
	<-d.done
	return d.conn.Close()
}

var writableTables = map[string]struct{}{
	// OTEL/observability ingest tables.
	"otel_logs":                     {},
	"otel_traces":                   {},
	"otel_metrics_gauge":            {},
	"otel_metrics_sum":              {},
	"otel_metrics_histogram":        {},
	"otel_metrics_gauge_pinned":     {},
	"otel_metrics_sum_pinned":       {},
	"otel_metrics_histogram_pinned": {},
	"hyperdx_sessions":              {},
	// SOBS internal state tables.
	"sobs_ai_memories":           {},
	"sobs_ai_settings":           {},
	"sobs_agent_rules":           {},
	"sobs_agent_runs":            {},
	"sobs_anomaly_rules":         {},
	"sobs_app_releases":          {},
	"sobs_app_settings":          {},
	"sobs_apps":                  {},
	"sobs_chart_configs":         {},
	"sobs_cve_dispositions":      {},
	"sobs_cve_findings":          {},
	"sobs_dashboards":            {},
	"sobs_github_work_items":     {},
	"sobs_log_attr_keys":         {},
	"sobs_notification_channels": {},
	"sobs_notification_log":      {},
	"sobs_notification_rules":    {},
	"sobs_raw_window_copy_state": {},
	"sobs_raw_windows":           {},
	"sobs_record_tags":           {},
	"sobs_release_artifacts":     {},
	"sobs_reports":               {},
	"sobs_tag_rules":             {},
}

func (d *DB) InsertJSONRows(table string, rows []map[string]any) error {
	if len(rows) == 0 {
		return nil
	}
	if !isWritableTable(table) {
		return fmt.Errorf("insert into %s: unsupported table", table)
	}

	normalized := normalizeJSONRows(rows)
	payload, err := marshalJSONEachRow(normalized)
	if err != nil {
		return err
	}

	if d.httpURL == "" {
		query := fmt.Sprintf("INSERT INTO %s FORMAT JSONEachRow\n%s", table, payload)
		if _, err := d.conn.ExecContext(context.Background(), query); err != nil {
			return fmt.Errorf("insert into %s: %w", table, err)
		}
		return nil
	}

	url := fmt.Sprintf("%s/?query=%s&date_time_input_format=best_effort",
		d.httpURL,
		neturl.QueryEscape(fmt.Sprintf("INSERT INTO %s FORMAT JSONEachRow", table)),
	)
	resp, err := httpClient.Post(url, "application/json", strings.NewReader(payload))
	if err != nil {
		return fmt.Errorf("insert into %s: %w", table, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("insert into %s: HTTP %d: %s", table, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func isWritableTable(table string) bool {
	_, ok := writableTables[table]
	return ok
}

func normalizeJSONRows(rows []map[string]any) []map[string]any {
	dtKeys := map[string]struct{}{
		"Timestamp":   {},
		"TimeUnix":    {},
		"UpdatedAt":   {},
		"CreatedAt":   {},
		"CompletedAt": {},
		"ReleasedAt":  {},
		"UploadedAt":  {},
		"ScannedAt":   {},
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := make(map[string]any, len(row))
		for k, v := range row {
			item[k] = v
		}
		for key := range dtKeys {
			if v, ok := item[key]; ok {
				item[key] = normalizeCHTimestamp(v)
			}
		}
		if events, ok := item["Events"].(map[string]any); ok {
			if ts, ok := events["Timestamp"]; ok {
				events["Timestamp"] = normalizeCHTimestamp(ts)
			}
		}
		out = append(out, item)
	}
	return out
}

func marshalJSONEachRow(rows []map[string]any) (string, error) {
	if len(rows) == 0 {
		return "", nil
	}
	var b strings.Builder
	for i, row := range rows {
		data, err := json.Marshal(row)
		if err != nil {
			return "", fmt.Errorf("marshal row: %w", err)
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		b.Write(data)
	}
	return b.String(), nil
}

func normalizeCHTimestamp(value any) string {
	if value == nil {
		return time.Now().UTC().Format("2006-01-02 15:04:05.000000")
	}
	switch v := value.(type) {
	case time.Time:
		return v.UTC().Format("2006-01-02 15:04:05.000000")
	default:
		raw := strings.TrimSpace(fmt.Sprint(value))
		if raw == "" || raw == "<nil>" {
			return time.Now().UTC().Format("2006-01-02 15:04:05.000000")
		}
		if t, err := time.Parse(time.RFC3339Nano, strings.ReplaceAll(raw, " ", "T")); err == nil {
			return t.UTC().Format("2006-01-02 15:04:05.000000")
		}
		return strings.ReplaceAll(raw, "T", " ")
	}
}

var httpClient = &nethttp.Client{Timeout: 30 * time.Second}

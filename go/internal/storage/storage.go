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

// InsertJSONRows inserts rows via ClickHouse HTTP API using JSONEachRow format.
func (d *DB) InsertJSONRows(table string, rows []map[string]any) error {
	if len(rows) == 0 {
		return nil
	}
	var buf strings.Builder
	for _, row := range rows {
		data, err := json.Marshal(row)
		if err != nil {
			return fmt.Errorf("marshal row: %w", err)
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}
	url := fmt.Sprintf("%s/?query=%s&date_time_input_format=best_effort",
		d.httpURL,
		neturl.QueryEscape(fmt.Sprintf("INSERT INTO %s FORMAT JSONEachRow", table)),
	)
	resp, err := httpClient.Post(url, "application/json", strings.NewReader(buf.String()))
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

var httpClient = &nethttp.Client{Timeout: 30 * time.Second}

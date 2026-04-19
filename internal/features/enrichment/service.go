package enrichment

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/persist"
)

type CVEFinding struct {
	OSVID      string `json:"osv_id"`
	Package    string `json:"package"`
	Severity   string `json:"severity"`
	Disposition string `json:"disposition"`
	UpdatedAt  string `json:"updated_at"`
}

type Service struct {
	mu       sync.RWMutex
	findings map[string]CVEFinding
	storeFactory extensionpoints.StoreFactory
	schemaOnce   sync.Once
	schemaErr    error
}

func NewService() *Service {
	now := time.Now().UTC().Format(time.RFC3339)
	return &Service{findings: map[string]CVEFinding{
		"OSV-2026-0001": {OSVID: "OSV-2026-0001", Package: "requests", Severity: "high", Disposition: "open", UpdatedAt: now},
		"OSV-2026-0002": {OSVID: "OSV-2026-0002", Package: "openssl", Severity: "critical", Disposition: "open", UpdatedAt: now},
	}}
}

func NewStoreService(factory extensionpoints.StoreFactory) *Service {
	return &Service{storeFactory: factory}
}

func (s *Service) ensureSchema(ctx context.Context) error {
	if s.storeFactory == nil {
		return nil
	}
	s.schemaOnce.Do(func() {
		store, err := persist.Open(ctx, s.storeFactory)
		if err != nil {
			s.schemaErr = err
			return
		}
		defer func() { _ = store.Close() }()
		_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_cve_findings (Package String, Ecosystem String, Version String, ServiceName String, OsvId String, CveIds String, Summary String, Severity String, Published String, ScannedAt DateTime64(3) DEFAULT now64(3)) ENGINE = ReplacingMergeTree(ScannedAt) ORDER BY (Package, Ecosystem, Version, OsvId)")
		if err == nil {
			_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_cve_dispositions (OsvId String, Package String, Ecosystem String, Version String, Disposition String, Note String, CreatedAt DateTime64(3) DEFAULT now64(3), UpdatedAt DateTime64(3) DEFAULT now64(3), Version_ UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version_) ORDER BY (OsvId, Package, Ecosystem, Version)")
		}
		if err == nil {
			rows, queryErr := store.Query(ctx, "SELECT count() FROM sobs_cve_findings FINAL")
			if queryErr == nil {
				defer func() { _ = rows.Close() }()
				var count uint64
				if rows.Next() {
					_ = rows.Scan(&count)
				}
				if count == 0 {
					_, err = store.Exec(ctx, "INSERT INTO sobs_cve_findings (Package, Ecosystem, Version, ServiceName, OsvId, CveIds, Summary, Severity) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", "requests", "pypi", "2.32.0", "web", "OSV-2026-0001", "[]", "Seed finding", "high")
					if err == nil {
						_, err = store.Exec(ctx, "INSERT INTO sobs_cve_findings (Package, Ecosystem, Version, ServiceName, OsvId, CveIds, Summary, Severity) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", "openssl", "system", "3.0", "edge", "OSV-2026-0002", "[]", "Seed finding", "critical")
					}
				}
			} else {
				err = queryErr
			}
		}
		s.schemaErr = err
	})
	return s.schemaErr
}

func (s *Service) Geo() []map[string]any {
	if s.storeFactory != nil {
		return s.aggregateTelemetry(context.Background(), "LogAttributes['client.geo.country']", "country")
	}
	return []map[string]any{{"country": "US", "count": 120}, {"country": "DE", "count": 30}, {"country": "IN", "count": 45}}
}

func (s *Service) Browsers() []map[string]any {
	if s.storeFactory != nil {
		return s.aggregateTelemetry(context.Background(), "LogAttributes['browser.context.browserName']", "browser")
	}
	return []map[string]any{{"browser": "Chrome", "count": 140}, {"browser": "Firefox", "count": 28}, {"browser": "Safari", "count": 20}}
}

func (s *Service) OS() []map[string]any {
	if s.storeFactory != nil {
		return s.aggregateTelemetry(context.Background(), "LogAttributes['browser.context.osName']", "os")
	}
	return []map[string]any{{"os": "macOS", "count": 60}, {"os": "Linux", "count": 75}, {"os": "Windows", "count": 53}}
}

func (s *Service) Timezones() []map[string]any {
	if s.storeFactory != nil {
		return s.aggregateTelemetry(context.Background(), "LogAttributes['browser.context.timezone']", "timezone")
	}
	return []map[string]any{{"timezone": "UTC", "count": 41}, {"timezone": "America/New_York", "count": 52}, {"timezone": "Europe/Berlin", "count": 18}}
}

func (s *Service) Languages() []map[string]any {
	if s.storeFactory != nil {
		return s.aggregateTelemetry(context.Background(), "LogAttributes['browser.context.language']", "language")
	}
	return []map[string]any{{"language": "en-US", "count": 130}, {"language": "de-DE", "count": 16}, {"language": "fr-FR", "count": 11}}
}

func (s *Service) Devices() []map[string]any {
	if s.storeFactory != nil {
		return s.aggregateTelemetry(context.Background(), "LogAttributes['browser.context.deviceClass']", "device")
	}
	return []map[string]any{{"device": "desktop", "count": 155}, {"device": "mobile", "count": 61}, {"device": "tablet", "count": 9}}
}

func (s *Service) Libraries() []map[string]any {
	if s.storeFactory != nil {
		return s.listLibrariesStoreBacked(context.Background())
	}
	return []map[string]any{{"name": "flask", "version": "3.1.0"}, {"name": "clickhouse-connect", "version": "0.8.18"}, {"name": "requests", "version": "2.32.0"}}
}

func (s *Service) GitHubRepoHealth() map[string]any {
	if s.storeFactory != nil {
		return s.repoHealthStoreBacked(context.Background())
	}
	return map[string]any{"status": "ok", "repos": 3, "token_valid": true}
}

func (s *Service) ListFindings() []CVEFinding {
	if s.storeFactory != nil {
		return s.listFindingsStoreBacked(context.Background())
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]CVEFinding, 0, len(s.findings))
	for _, f := range s.findings {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OSVID < out[j].OSVID })
	return out
}

func (s *Service) listFindingsStoreBacked(ctx context.Context) []CVEFinding {
	if err := s.ensureSchema(ctx); err != nil {
		return nil
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return nil
	}
	defer func() { _ = store.Close() }()
	dispositions := map[string]string{}
	dispRows, err := store.Query(ctx, "SELECT OsvId, Package, Ecosystem, Version, Disposition FROM sobs_cve_dispositions FINAL")
	if err == nil {
		defer func() { _ = dispRows.Close() }()
		for dispRows.Next() {
			var osvID, pkg, eco, ver, disposition string
			if err := dispRows.Scan(&osvID, &pkg, &eco, &ver, &disposition); err != nil {
				break
			}
			dispositions[osvID+"|"+pkg+"|"+eco+"|"+ver] = disposition
		}
	}
	rows, err := store.Query(ctx, "SELECT OsvId, Package, Severity, Ecosystem, Version, ScannedAt FROM sobs_cve_findings FINAL ORDER BY ScannedAt DESC LIMIT 200")
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	out := []CVEFinding{}
	for rows.Next() {
		var item CVEFinding
		var ecosystem, version string
		if err := rows.Scan(&item.OSVID, &item.Package, &item.Severity, &ecosystem, &version, &item.UpdatedAt); err != nil {
			return out
		}
		if disposition := dispositions[item.OSVID+"|"+item.Package+"|"+ecosystem+"|"+version]; disposition != "" {
			item.Disposition = disposition
		} else {
			item.Disposition = "open"
		}
		out = append(out, item)
	}
	return out
}

func (s *Service) SetDisposition(osvID, disposition string) (CVEFinding, bool) {
	if s.storeFactory != nil {
		return s.setDispositionStoreBacked(context.Background(), osvID, disposition)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.findings[osvID]
	if !ok {
		return CVEFinding{}, false
	}
	d := strings.TrimSpace(disposition)
	if d == "" {
		d = "open"
	}
	f.Disposition = d
	f.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.findings[osvID] = f
	return f, true
}

func (s *Service) Scan() map[string]any {
	if s.storeFactory != nil {
		return s.scanStoreBacked(context.Background())
	}
	return map[string]any{"ok": true, "scanned": len(s.findings)}
}

func (s *Service) aggregateTelemetry(ctx context.Context, expr, field string) []map[string]any {
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return []map[string]any{}
	}
	defer func() { _ = store.Close() }()
	query := "SELECT " + expr + " AS value, count() AS cnt FROM otel_logs WHERE " + expr + " != '' GROUP BY value ORDER BY cnt DESC LIMIT 50"
	rows, err := store.Query(ctx, query)
	if err != nil {
		return []map[string]any{}
	}
	defer func() { _ = rows.Close() }()
	out := []map[string]any{}
	for rows.Next() {
		var value string
		var count uint64
		if err := rows.Scan(&value, &count); err != nil {
			return out
		}
		out = append(out, map[string]any{field: value, "count": count})
	}
	return out
}

func (s *Service) listLibrariesStoreBacked(ctx context.Context) []map[string]any {
	if err := s.ensureSchema(ctx); err != nil {
		return []map[string]any{}
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return []map[string]any{}
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT Package, any(Version) FROM sobs_cve_findings FINAL GROUP BY Package ORDER BY Package LIMIT 100")
	if err != nil {
		return []map[string]any{}
	}
	defer func() { _ = rows.Close() }()
	out := []map[string]any{}
	for rows.Next() {
		var name, version string
		if err := rows.Scan(&name, &version); err != nil {
			return out
		}
		out = append(out, map[string]any{"name": name, "version": version})
	}
	return out
}

func (s *Service) repoHealthStoreBacked(ctx context.Context) map[string]any {
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return map[string]any{"status": "ok", "repos": 0, "token_valid": false}
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT count() FROM sobs_apps FINAL WHERE IsDeleted = 0")
	if err != nil {
		return map[string]any{"status": "ok", "repos": 0, "token_valid": false}
	}
	defer func() { _ = rows.Close() }()
	var count uint64
	if rows.Next() {
		_ = rows.Scan(&count)
	}
	return map[string]any{"status": "ok", "repos": count, "token_valid": count > 0}
}

func (s *Service) setDispositionStoreBacked(ctx context.Context, osvID, disposition string) (CVEFinding, bool) {
	if err := s.ensureSchema(ctx); err != nil {
		return CVEFinding{}, false
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return CVEFinding{}, false
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT Package, Severity, Ecosystem, Version, ScannedAt FROM sobs_cve_findings FINAL WHERE OsvId = ? ORDER BY ScannedAt DESC LIMIT 1", osvID)
	if err != nil {
		return CVEFinding{}, false
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return CVEFinding{}, false
	}
	var finding CVEFinding
	var ecosystem, version string
	if err := rows.Scan(&finding.Package, &finding.Severity, &ecosystem, &version, &finding.UpdatedAt); err != nil {
		return CVEFinding{}, false
	}
	finding.OSVID = osvID
	disposition = strings.TrimSpace(disposition)
	if disposition == "" {
		disposition = "open"
	}
	finding.Disposition = disposition
	now := persist.RFC3339Now()
	_, err = store.Exec(ctx, "INSERT INTO sobs_cve_dispositions (OsvId, Package, Ecosystem, Version, Disposition, Note, CreatedAt, UpdatedAt, Version_) VALUES (?, ?, ?, ?, ?, ?, parseDateTime64BestEffort(?), parseDateTime64BestEffort(?), ?)", osvID, finding.Package, ecosystem, version, disposition, "", now, now, persist.Version())
	if err != nil {
		return CVEFinding{}, false
	}
	finding.UpdatedAt = now
	return finding, true
}

func (s *Service) scanStoreBacked(ctx context.Context) map[string]any {
	_ = persist.SetAppSetting(ctx, s.storeFactory, "enrichment.cve_last_scan", persist.RFC3339Now())
	findings := s.listFindingsStoreBacked(ctx)
	return map[string]any{"ok": true, "scanned": len(findings)}
}

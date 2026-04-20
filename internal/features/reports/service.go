package reports

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/defaultstore"
	"github.com/abartrim/sobs/internal/features/persist"
)

type Report struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	PageType    string         `json:"page_type"`
	Filters     map[string]any `json:"filters"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
}

var allowedPageTypes = map[string]struct{}{
	"logs":        {},
	"traces":      {},
	"errors":      {},
	"metrics":     {},
	"rum":         {},
	"ai":          {},
	"work_items":  {},
	"web_traffic": {},
}

type Service struct {
	mu           sync.RWMutex
	reports      map[string]Report
	nextID       int64
	storeFactory extensionpoints.StoreFactory
	schemaOnce   sync.Once
	schemaErr    error
}

func NewService() *Service {
	return NewStoreService(defaultstore.NewFactory())
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
		_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_reports (Id String CODEC(ZSTD(1)), Name String CODEC(ZSTD(1)), Description String CODEC(ZSTD(1)), PageType LowCardinality(String) CODEC(ZSTD(1)), FiltersJson String CODEC(ZSTD(1)), IsDeleted UInt8 DEFAULT 0 CODEC(T64, ZSTD(1)), Version UInt64 DEFAULT 0 CODEC(T64, ZSTD(1))) ENGINE = ReplacingMergeTree(Version) ORDER BY Id SETTINGS index_granularity = 8192")
		s.schemaErr = err
	})
	return s.schemaErr
}

func (s *Service) List() []Report {
	return s.listStoreBacked(context.Background(), "")
}

func (s *Service) ListByPageType(pageType string) []Report {
	return s.listStoreBacked(context.Background(), strings.TrimSpace(pageType))
}

func (s *Service) listStoreBacked(ctx context.Context, pageType string) []Report {
	if err := s.ensureSchema(ctx); err != nil {
		return nil
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return nil
	}
	defer func() { _ = store.Close() }()
	query := "SELECT Id, Name, Description, PageType, FiltersJson, Version FROM sobs_reports FINAL WHERE IsDeleted = 0"
	args := []any{}
	if pageType != "" {
		query += " AND PageType = ?"
		args = append(args, pageType)
		query += " ORDER BY Name"
	} else {
		query += " ORDER BY PageType, Name"
	}
	rows, err := store.Query(ctx, query, args...)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	out := []Report{}
	for rows.Next() {
		var item Report
		var description string
		var pageTypeValue string
		var filtersJSON string
		var version uint64
		if err := rows.Scan(&item.ID, &item.Name, &description, &pageTypeValue, &filtersJSON, &version); err != nil {
			return out
		}
		item.Description = description
		item.PageType = pageTypeValue
		item.Filters = parseReportFilters(filtersJSON)
		item.CreatedAt = time.Unix(0, int64(version)).UTC().Format(time.RFC3339)
		item.UpdatedAt = item.CreatedAt
		out = append(out, item)
	}
	return out
}

func (s *Service) Create(report Report) (Report, error) {
	return s.createStoreBacked(context.Background(), report)
}

func (s *Service) createStoreBacked(ctx context.Context, report Report) (Report, error) {
	if strings.TrimSpace(report.Name) == "" {
		return Report{}, errors.New("name is required")
	}
	if _, ok := allowedPageTypes[strings.TrimSpace(report.PageType)]; !ok {
		return Report{}, errors.New("page_type is invalid")
	}
	if report.Filters == nil {
		report.Filters = map[string]any{}
	}
	if err := s.ensureSchema(ctx); err != nil {
		return Report{}, err
	}
	id := persist.NewID()
	version := persist.Version()
	createdAt := time.Unix(0, int64(version)).UTC().Format(time.RFC3339)
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Report{}, err
	}
	defer func() { _ = store.Close() }()
	if _, err := store.Exec(ctx, "INSERT INTO sobs_reports (Id, Name, Description, PageType, FiltersJson, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?)", id, strings.TrimSpace(report.Name), strings.TrimSpace(report.Description), strings.TrimSpace(report.PageType), persist.JSONString(report.Filters), 0, version); err != nil {
		return Report{}, err
	}
	report.ID = id
	report.CreatedAt = createdAt
	report.UpdatedAt = createdAt
	return report, nil
}

func (s *Service) Delete(id string) bool {
	return s.deleteStoreBacked(context.Background(), id)
}

func (s *Service) deleteStoreBacked(ctx context.Context, id string) bool {
	if err := s.ensureSchema(ctx); err != nil {
		return false
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return false
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT Name, Description, PageType, FiltersJson FROM sobs_reports FINAL WHERE IsDeleted = 0 AND Id = ? LIMIT 1", id)
	if err != nil {
		return false
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return false
	}
	var name string
	var description string
	var pageType string
	var filtersJSON string
	if err := rows.Scan(&name, &description, &pageType, &filtersJSON); err != nil {
		return false
	}
	_, err = store.Exec(ctx, "INSERT INTO sobs_reports (Id, Name, Description, PageType, FiltersJson, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?)", id, name, description, pageType, filtersJSON, 1, persist.Version())
	return err == nil
}

func (s *Service) ReplaceAll(in []Report) {
	s.replaceAllStoreBacked(context.Background(), in)
}

func (s *Service) replaceAllStoreBacked(ctx context.Context, in []Report) {
	if err := s.ensureSchema(ctx); err != nil {
		return
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return
	}
	defer func() { _ = store.Close() }()
	existingRows, err := store.Query(ctx, "SELECT Id, Name, Description, PageType, FiltersJson FROM sobs_reports FINAL WHERE IsDeleted = 0 ORDER BY PageType, Name")
	if err == nil {
		defer func() { _ = existingRows.Close() }()
		for existingRows.Next() {
			var id, name, description, pageType, filtersJSON string
			if scanErr := existingRows.Scan(&id, &name, &description, &pageType, &filtersJSON); scanErr != nil {
				continue
			}
			_, _ = store.Exec(ctx, "INSERT INTO sobs_reports (Id, Name, Description, PageType, FiltersJson, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?)", id, name, description, pageType, filtersJSON, 1, persist.Version())
		}
	}
	for _, item := range in {
		if _, ok := allowedPageTypes[strings.TrimSpace(item.PageType)]; !ok {
			continue
		}
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = persist.NewID()
		}
		filters := item.Filters
		if filters == nil {
			filters = map[string]any{}
		}
		_, _ = store.Exec(ctx, "INSERT INTO sobs_reports (Id, Name, Description, PageType, FiltersJson, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?)", id, strings.TrimSpace(item.Name), strings.TrimSpace(item.Description), strings.TrimSpace(item.PageType), persist.JSONString(filters), 0, persist.Version())
	}
}

func parseReportFilters(raw string) map[string]any {
	parsed := persist.ParseJSONMap(raw)
	if parsed == nil {
		return map[string]any{}
	}
	return parsed
}

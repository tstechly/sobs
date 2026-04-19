package reports

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/persist"
)

type Report struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Query     string `json:"query"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type Service struct {
	mu      sync.RWMutex
	reports map[string]Report
	nextID  int64
	storeFactory extensionpoints.StoreFactory
	schemaOnce   sync.Once
	schemaErr    error
}

func NewService() *Service {
	return &Service{reports: make(map[string]Report), nextID: 1}
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
		_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_reports (Id String, Name String, Description String, PageType String, FiltersJson String, IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version) ORDER BY Id")
		s.schemaErr = err
	})
	return s.schemaErr
}

func (s *Service) List() []Report {
	if s.storeFactory != nil {
		return s.listStoreBacked(context.Background())
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Report, 0, len(s.reports))
	for _, r := range s.reports {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Service) listStoreBacked(ctx context.Context) []Report {
	if err := s.ensureSchema(ctx); err != nil {
		return nil
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return nil
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT Id, Name, Description, FiltersJson, Version FROM sobs_reports FINAL WHERE IsDeleted = 0 ORDER BY Name")
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	out := []Report{}
	for rows.Next() {
		var item Report
		var description string
		var filtersJSON string
		var version uint64
		if err := rows.Scan(&item.ID, &item.Name, &description, &filtersJSON, &version); err != nil {
			return out
		}
		item.Query = filtersJSON
		item.CreatedAt = time.Unix(0, int64(version)).UTC().Format(time.RFC3339)
		item.UpdatedAt = item.CreatedAt
		out = append(out, item)
	}
	return out
}

func (s *Service) Create(name, query string) (Report, error) {
	if s.storeFactory != nil {
		return s.createStoreBacked(context.Background(), name, query)
	}
	if name == "" {
		return Report{}, errors.New("name is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	s.mu.Lock()
	defer s.mu.Unlock()
	id := strconv.FormatInt(s.nextID, 10)
	s.nextID++
	r := Report{ID: id, Name: name, Query: query, CreatedAt: now, UpdatedAt: now}
	s.reports[id] = r
	return r, nil
}

func (s *Service) createStoreBacked(ctx context.Context, name string, query string) (Report, error) {
	if name == "" {
		return Report{}, errors.New("name is required")
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
	if _, err := store.Exec(ctx, "INSERT INTO sobs_reports (Id, Name, Description, PageType, FiltersJson, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?)", id, name, "", "query", query, 0, version); err != nil {
		return Report{}, err
	}
	return Report{ID: id, Name: name, Query: query, CreatedAt: createdAt, UpdatedAt: createdAt}, nil
}

func (s *Service) Delete(id string) bool {
	if s.storeFactory != nil {
		return s.deleteStoreBacked(context.Background(), id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.reports[id]; !ok {
		return false
	}
	delete(s.reports, id)
	return true
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
	if s.storeFactory != nil {
		s.replaceAllStoreBacked(context.Background(), in)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reports = make(map[string]Report, len(in))
	var maxID int64
	for _, r := range in {
		s.reports[r.ID] = r
		if n, err := strconv.ParseInt(r.ID, 10, 64); err == nil && n > maxID {
			maxID = n
		}
	}
	s.nextID = maxID + 1
	if s.nextID < 1 {
		s.nextID = 1
	}
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
	for _, item := range s.listStoreBacked(ctx) {
		_, _ = store.Exec(ctx, "INSERT INTO sobs_reports (Id, Name, Description, PageType, FiltersJson, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?)", item.ID, item.Name, "", "query", item.Query, 1, persist.Version())
	}
	for _, item := range in {
		id := item.ID
		if id == "" {
			id = persist.NewID()
		}
		_, _ = store.Exec(ctx, "INSERT INTO sobs_reports (Id, Name, Description, PageType, FiltersJson, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?)", id, item.Name, "", "query", item.Query, 0, persist.Version())
	}
}

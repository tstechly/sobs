package dashboards

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abartrim/sobs/internal/features/defaultstore"
	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/persist"
)

type Dashboard struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Description string `json:"description"`
	CreatedAt string `json:"created_at"`
}

type Chart struct {
	ID        string         `json:"id"`
	DashboardID string       `json:"dashboard_id"`
	Title     string         `json:"title"`
	Type      string         `json:"type"`
	Spec      map[string]any `json:"spec"`
	CreatedAt string         `json:"created_at"`
}

type QueryResult struct {
	Columns []string        `json:"columns"`
	Rows    [][]interface{} `json:"rows"`
}

type Service struct {
	mu          sync.RWMutex
	items       map[string]Dashboard
	chartsByDash map[string]map[string]Chart
	nextID      int64
	nextChartID int64
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
		_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_dashboards (Id String, Name String, Description String, IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version) ORDER BY Id")
		if err == nil {
			_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_chart_configs (Id String, DashboardId String, Title String, ChartType String, Query String, OptionsJson String, Position UInt16 DEFAULT 0, IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version) ORDER BY (DashboardId, Id)")
		}
		if err == nil {
			rows, queryErr := store.Query(ctx, "SELECT count() FROM sobs_dashboards FINAL WHERE IsDeleted = 0")
			if queryErr == nil {
				defer func() { _ = rows.Close() }()
				var count uint64
				if rows.Next() {
					_ = rows.Scan(&count)
				}
				if count == 0 {
					_, err = store.Exec(ctx, "INSERT INTO sobs_dashboards (Id, Name, Description, IsDeleted, Version) VALUES (?, ?, ?, ?, ?)", "1", "Default Dashboard", "Seed dashboard", 0, persist.Version())
				}
			} else {
				err = queryErr
			}
		}
		s.schemaErr = err
	})
	return s.schemaErr
}

func (s *Service) seed() {
	now := time.Now().UTC().Format(time.RFC3339)
	s.items["1"] = Dashboard{ID: "1", Name: "Default Dashboard", Description: "Seed dashboard", CreatedAt: now}
	s.chartsByDash["1"] = map[string]Chart{}
	s.nextID = 2
	s.nextChartID = 1
}

func (s *Service) List() []Dashboard {
	if s.storeFactory != nil {
		return s.listStoreBacked(context.Background())
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Dashboard, 0, len(s.items))
	for _, d := range s.items {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Service) listStoreBacked(ctx context.Context) []Dashboard {
	if err := s.ensureSchema(ctx); err != nil {
		return nil
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return nil
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT Id, Name, Description, Version FROM sobs_dashboards FINAL WHERE IsDeleted = 0 ORDER BY Name")
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	out := []Dashboard{}
	for rows.Next() {
		var item Dashboard
		var version uint64
		if err := rows.Scan(&item.ID, &item.Name, &item.Description, &version); err != nil {
			return out
		}
		item.CreatedAt = time.Unix(0, int64(version)).UTC().Format(time.RFC3339)
		out = append(out, item)
	}
	return out
}

func (s *Service) Create(name, description string) (Dashboard, error) {
	if s.storeFactory != nil {
		return s.createStoreBacked(context.Background(), name, description)
	}
	if strings.TrimSpace(name) == "" {
		return Dashboard{}, errors.New("name is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := strconv.FormatInt(s.nextID, 10)
	s.nextID++
	d := Dashboard{ID: id, Name: strings.TrimSpace(name), Description: strings.TrimSpace(description), CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	s.items[id] = d
	s.chartsByDash[id] = map[string]Chart{}
	return d, nil
}

func (s *Service) createStoreBacked(ctx context.Context, name, description string) (Dashboard, error) {
	if strings.TrimSpace(name) == "" {
		return Dashboard{}, errors.New("name is required")
	}
	if err := s.ensureSchema(ctx); err != nil {
		return Dashboard{}, err
	}
	id := persist.NewID()
	version := persist.Version()
	createdAt := time.Unix(0, int64(version)).UTC().Format(time.RFC3339)
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Dashboard{}, err
	}
	defer func() { _ = store.Close() }()
	_, err = store.Exec(ctx, "INSERT INTO sobs_dashboards (Id, Name, Description, IsDeleted, Version) VALUES (?, ?, ?, ?, ?)", id, strings.TrimSpace(name), strings.TrimSpace(description), 0, version)
	if err != nil {
		return Dashboard{}, err
	}
	return Dashboard{ID: id, Name: strings.TrimSpace(name), Description: strings.TrimSpace(description), CreatedAt: createdAt}, nil
}

func (s *Service) Get(id string) (Dashboard, bool) {
	if s.storeFactory != nil {
		return s.getStoreBacked(context.Background(), id)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.items[id]
	return d, ok
}

func (s *Service) getStoreBacked(ctx context.Context, id string) (Dashboard, bool) {
	if err := s.ensureSchema(ctx); err != nil {
		return Dashboard{}, false
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Dashboard{}, false
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT Id, Name, Description, Version FROM sobs_dashboards FINAL WHERE IsDeleted = 0 AND Id = ? LIMIT 1", id)
	if err != nil {
		return Dashboard{}, false
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return Dashboard{}, false
	}
	var item Dashboard
	var version uint64
	if err := rows.Scan(&item.ID, &item.Name, &item.Description, &version); err != nil {
		return Dashboard{}, false
	}
	item.CreatedAt = time.Unix(0, int64(version)).UTC().Format(time.RFC3339)
	return item, true
}

func (s *Service) Delete(id string) bool {
	if s.storeFactory != nil {
		return s.deleteStoreBacked(context.Background(), id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; !ok {
		return false
	}
	delete(s.items, id)
	delete(s.chartsByDash, id)
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
	rows, err := store.Query(ctx, "SELECT Name, Description FROM sobs_dashboards FINAL WHERE IsDeleted = 0 AND Id = ? LIMIT 1", id)
	if err != nil {
		return false
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return false
	}
	var name, description string
	if err := rows.Scan(&name, &description); err != nil {
		return false
	}
	_, err = store.Exec(ctx, "INSERT INTO sobs_dashboards (Id, Name, Description, IsDeleted, Version) VALUES (?, ?, ?, ?, ?)", id, name, description, 1, persist.Version())
	return err == nil
}

func (s *Service) AddChart(dashboardID, title, chartType string, spec map[string]any) (Chart, error) {
	if s.storeFactory != nil {
		return s.addChartStoreBacked(context.Background(), dashboardID, title, chartType, spec)
	}
	if strings.TrimSpace(title) == "" {
		return Chart{}, errors.New("title is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[dashboardID]; !ok {
		return Chart{}, errors.New("dashboard not found")
	}
	if _, ok := s.chartsByDash[dashboardID]; !ok {
		s.chartsByDash[dashboardID] = map[string]Chart{}
	}
	id := strconv.FormatInt(s.nextChartID, 10)
	s.nextChartID++
	if strings.TrimSpace(chartType) == "" {
		chartType = "line"
	}
	if spec == nil {
		spec = map[string]any{}
	}
	c := Chart{ID: id, DashboardID: dashboardID, Title: strings.TrimSpace(title), Type: strings.TrimSpace(chartType), Spec: spec, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	s.chartsByDash[dashboardID][id] = c
	return c, nil
}

func (s *Service) addChartStoreBacked(ctx context.Context, dashboardID, title, chartType string, spec map[string]any) (Chart, error) {
	if strings.TrimSpace(title) == "" {
		return Chart{}, errors.New("title is required")
	}
	if _, ok := s.getStoreBacked(ctx, dashboardID); !ok {
		return Chart{}, errors.New("dashboard not found")
	}
	if strings.TrimSpace(chartType) == "" {
		chartType = "line"
	}
	if spec == nil {
		spec = map[string]any{}
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Chart{}, err
	}
	defer func() { _ = store.Close() }()
	id := persist.NewID()
	version := persist.Version()
	createdAt := time.Unix(0, int64(version)).UTC().Format(time.RFC3339)
	query, _ := spec["query"].(string)
	_, err = store.Exec(ctx, "INSERT INTO sobs_chart_configs (Id, DashboardId, Title, ChartType, Query, OptionsJson, Position, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", id, dashboardID, strings.TrimSpace(title), strings.TrimSpace(chartType), query, persist.JSONString(spec), 0, 0, version)
	if err != nil {
		return Chart{}, err
	}
	return Chart{ID: id, DashboardID: dashboardID, Title: strings.TrimSpace(title), Type: strings.TrimSpace(chartType), Spec: spec, CreatedAt: createdAt}, nil
}

func (s *Service) EditChart(dashboardID, chartID, title, chartType string, spec map[string]any) (Chart, bool) {
	if s.storeFactory != nil {
		return s.editChartStoreBacked(context.Background(), dashboardID, chartID, title, chartType, spec)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	charts, ok := s.chartsByDash[dashboardID]
	if !ok {
		return Chart{}, false
	}
	c, ok := charts[chartID]
	if !ok {
		return Chart{}, false
	}
	if strings.TrimSpace(title) != "" {
		c.Title = strings.TrimSpace(title)
	}
	if strings.TrimSpace(chartType) != "" {
		c.Type = strings.TrimSpace(chartType)
	}
	if spec != nil {
		c.Spec = spec
	}
	charts[chartID] = c
	return c, true
}

func (s *Service) editChartStoreBacked(ctx context.Context, dashboardID, chartID, title, chartType string, spec map[string]any) (Chart, bool) {
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Chart{}, false
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT Title, ChartType, Query, OptionsJson FROM sobs_chart_configs FINAL WHERE IsDeleted = 0 AND DashboardId = ? AND Id = ? LIMIT 1", dashboardID, chartID)
	if err != nil {
		return Chart{}, false
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return Chart{}, false
	}
	var currentTitle, currentType, query, optionsJSON string
	if err := rows.Scan(&currentTitle, &currentType, &query, &optionsJSON); err != nil {
		return Chart{}, false
	}
	if strings.TrimSpace(title) != "" {
		currentTitle = strings.TrimSpace(title)
	}
	if strings.TrimSpace(chartType) != "" {
		currentType = strings.TrimSpace(chartType)
	}
	if spec == nil {
		spec = persist.ParseJSONMap(optionsJSON)
	} else if q, ok := spec["query"].(string); ok {
		query = q
	}
	_, err = store.Exec(ctx, "INSERT INTO sobs_chart_configs (Id, DashboardId, Title, ChartType, Query, OptionsJson, Position, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", chartID, dashboardID, currentTitle, currentType, query, persist.JSONString(spec), 0, 0, persist.Version())
	if err != nil {
		return Chart{}, false
	}
	return Chart{ID: chartID, DashboardID: dashboardID, Title: currentTitle, Type: currentType, Spec: spec, CreatedAt: persist.RFC3339Now()}, true
}

func (s *Service) CloneChart(dashboardID, chartID string) (Chart, bool) {
	if s.storeFactory != nil {
		chart, ok := s.ExportChart(dashboardID, chartID)
		if !ok {
			return Chart{}, false
		}
		clone, err := s.addChartStoreBacked(context.Background(), dashboardID, chart.Title+" (Copy)", chart.Type, chart.Spec)
		return clone, err == nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	charts, ok := s.chartsByDash[dashboardID]
	if !ok {
		return Chart{}, false
	}
	c, ok := charts[chartID]
	if !ok {
		return Chart{}, false
	}
	id := strconv.FormatInt(s.nextChartID, 10)
	s.nextChartID++
	clone := c
	clone.ID = id
	clone.Title = c.Title + " (Copy)"
	clone.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	charts[id] = clone
	return clone, true
}

func (s *Service) DeleteChart(dashboardID, chartID string) bool {
	if s.storeFactory != nil {
		return s.deleteChartStoreBacked(context.Background(), dashboardID, chartID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	charts, ok := s.chartsByDash[dashboardID]
	if !ok {
		return false
	}
	if _, ok := charts[chartID]; !ok {
		return false
	}
	delete(charts, chartID)
	return true
}

func (s *Service) deleteChartStoreBacked(ctx context.Context, dashboardID, chartID string) bool {
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return false
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT Title, ChartType, Query, OptionsJson FROM sobs_chart_configs FINAL WHERE IsDeleted = 0 AND DashboardId = ? AND Id = ? LIMIT 1", dashboardID, chartID)
	if err != nil {
		return false
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return false
	}
	var title, chartType, query, optionsJSON string
	if err := rows.Scan(&title, &chartType, &query, &optionsJSON); err != nil {
		return false
	}
	_, err = store.Exec(ctx, "INSERT INTO sobs_chart_configs (Id, DashboardId, Title, ChartType, Query, OptionsJson, Position, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", chartID, dashboardID, title, chartType, query, optionsJSON, 0, 1, persist.Version())
	return err == nil
}

func (s *Service) ExportChart(dashboardID, chartID string) (Chart, bool) {
	if s.storeFactory != nil {
		return s.exportChartStoreBacked(context.Background(), dashboardID, chartID)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	charts, ok := s.chartsByDash[dashboardID]
	if !ok {
		return Chart{}, false
	}
	c, ok := charts[chartID]
	return c, ok
}

func (s *Service) exportChartStoreBacked(ctx context.Context, dashboardID, chartID string) (Chart, bool) {
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Chart{}, false
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT Id, DashboardId, Title, ChartType, OptionsJson, Version FROM sobs_chart_configs FINAL WHERE IsDeleted = 0 AND DashboardId = ? AND Id = ? LIMIT 1", dashboardID, chartID)
	if err != nil {
		return Chart{}, false
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return Chart{}, false
	}
	var chart Chart
	var optionsJSON string
	var version uint64
	if err := rows.Scan(&chart.ID, &chart.DashboardID, &chart.Title, &chart.Type, &optionsJSON, &version); err != nil {
		return Chart{}, false
	}
	chart.Spec = persist.ParseJSONMap(optionsJSON)
	chart.CreatedAt = time.Unix(0, int64(version)).UTC().Format(time.RFC3339)
	return chart, true
}

func (s *Service) ImportCharts(dashboardID string, charts []Chart) int {
	if s.storeFactory != nil {
		count := 0
		for _, chart := range charts {
			if _, err := s.addChartStoreBacked(context.Background(), dashboardID, chart.Title, chart.Type, chart.Spec); err == nil {
				count++
			}
		}
		return count
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[dashboardID]; !ok {
		return 0
	}
	if _, ok := s.chartsByDash[dashboardID]; !ok {
		s.chartsByDash[dashboardID] = map[string]Chart{}
	}
	count := 0
	for _, in := range charts {
		id := strconv.FormatInt(s.nextChartID, 10)
		s.nextChartID++
		if strings.TrimSpace(in.Title) == "" {
			in.Title = "Imported Chart"
		}
		if strings.TrimSpace(in.Type) == "" {
			in.Type = "line"
		}
		in.ID = id
		in.DashboardID = dashboardID
		in.CreatedAt = time.Now().UTC().Format(time.RFC3339)
		s.chartsByDash[dashboardID][id] = in
		count++
	}
	return count
}

func (s *Service) SpecTemplates() []map[string]any {
	return []map[string]any{{"id": "latency-line", "label": "Latency Line"}, {"id": "error-rate", "label": "Error Rate"}}
}

func (s *Service) SpecOptions() map[string]any {
	return map[string]any{"chart_types": []string{"line", "bar", "area", "pie", "table"}, "time_windows": []string{"15m", "1h", "24h"}}
}

func (s *Service) BuildSpec(prompt string) map[string]any {
	return map[string]any{"title": strings.TrimSpace(prompt), "type": "line", "query": "SELECT Timestamp, Value FROM sobs_metrics FINAL LIMIT 100"}
}

func (s *Service) ValidateSpec(spec map[string]any) (bool, string) {
	if spec == nil {
		return false, "spec is required"
	}
	t, _ := spec["type"].(string)
	if strings.TrimSpace(t) == "" {
		return false, "type is required"
	}
	return true, ""
}

func (s *Service) RenderSpec(spec map[string]any) map[string]any {
	return map[string]any{"ok": true, "spec": spec, "series": []map[string]any{{"name": "value", "data": []int{1, 2, 3}}}}
}

func (s *Service) Query(sql string) QueryResult {
	_ = sql
	return QueryResult{
		Columns: []string{"status", "count"},
		Rows:    [][]interface{}{{"ok", 1}},
	}
}

package repositories

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/persist"
)

type Repository struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	URL         string   `json:"url"`
	Realtime    bool     `json:"realtime"`
	CIIngestKey string   `json:"ci_ingest_key"`
	Releases    []string `json:"releases"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

type Service struct {
	mu      sync.RWMutex
	items   map[string]Repository
	nextID  int64
	storeFactory extensionpoints.StoreFactory
	schemaOnce   sync.Once
	schemaErr    error
}

func NewService() *Service {
	return &Service{items: make(map[string]Repository), nextID: 1}
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
		_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_apps (Id String, Name String, Slug String, OwnerTeam String, RepoUrl String, DefaultEnvironment String, Enabled UInt8 DEFAULT 1, MetadataJson String, IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0, CreatedAt DateTime64(3) DEFAULT now64(3), UpdatedAt DateTime64(3) DEFAULT now64(3)) ENGINE = ReplacingMergeTree(Version) ORDER BY (Slug, Id)")
		if err == nil {
			_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_app_releases (Id String, AppId String, ReleaseVersion String, CommitSha String, BuildId String, Environment String, ReleasedAt DateTime64(3) DEFAULT now64(3), MetadataJson String, IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version) ORDER BY (AppId, ReleaseVersion, Id)")
		}
		s.schemaErr = err
	})
	return s.schemaErr
}

func (s *Service) List() []Repository {
	if s.storeFactory != nil {
		return s.listStoreBacked(context.Background())
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Repository, 0, len(s.items))
	for _, r := range s.items {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Service) listStoreBacked(ctx context.Context) []Repository {
	if err := s.ensureSchema(ctx); err != nil {
		return nil
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return nil
	}
	defer func() { _ = store.Close() }()
	releasesByApp := map[string][]string{}
	releaseRows, err := store.Query(ctx, "SELECT AppId, ReleaseVersion FROM sobs_app_releases FINAL WHERE IsDeleted = 0 ORDER BY ReleasedAt DESC")
	if err == nil {
		defer func() { _ = releaseRows.Close() }()
		for releaseRows.Next() {
			var appID, version string
			if err := releaseRows.Scan(&appID, &version); err != nil {
				break
			}
			releasesByApp[appID] = append(releasesByApp[appID], version)
		}
	}
	rows, err := store.Query(ctx, "SELECT Id, Name, RepoUrl, MetadataJson, CreatedAt, UpdatedAt FROM sobs_apps FINAL WHERE IsDeleted = 0 ORDER BY Name")
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	out := []Repository{}
	for rows.Next() {
		var item Repository
		var metadataJSON string
		if err := rows.Scan(&item.ID, &item.Name, &item.URL, &metadataJSON, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return out
		}
		meta := persist.ParseJSONMap(metadataJSON)
		item.Realtime = asBool(meta["realtime"])
		item.CIIngestKey, _ = meta["ci_ingest_key"].(string)
		item.Releases = releasesByApp[item.ID]
		out = append(out, item)
	}
	return out
}

func (s *Service) Create(name, url string) (Repository, error) {
	if s.storeFactory != nil {
		return s.createStoreBacked(context.Background(), name, url)
	}
	if strings.TrimSpace(name) == "" {
		return Repository{}, errors.New("name is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	s.mu.Lock()
	defer s.mu.Unlock()
	id := strconv.FormatInt(s.nextID, 10)
	s.nextID++
	r := Repository{ID: id, Name: name, URL: url, Realtime: false, CIIngestKey: newSecret(), Releases: []string{}, CreatedAt: now, UpdatedAt: now}
	s.items[id] = r
	return r, nil
}

func (s *Service) createStoreBacked(ctx context.Context, name, url string) (Repository, error) {
	if strings.TrimSpace(name) == "" {
		return Repository{}, errors.New("name is required")
	}
	if err := s.ensureSchema(ctx); err != nil {
		return Repository{}, err
	}
	id := persist.NewID()
	now := persist.RFC3339Now()
	ciKey := newSecret()
	metadata := persist.JSONString(map[string]any{"realtime": false, "ci_ingest_key": ciKey})
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Repository{}, err
	}
	defer func() { _ = store.Close() }()
	_, err = store.Exec(ctx, "INSERT INTO sobs_apps (Id, Name, Slug, OwnerTeam, RepoUrl, DefaultEnvironment, Enabled, MetadataJson, IsDeleted, Version, CreatedAt, UpdatedAt) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, parseDateTime64BestEffort(?), parseDateTime64BestEffort(?))", id, strings.TrimSpace(name), id, "", strings.TrimSpace(url), "default", 1, metadata, 0, persist.Version(), now, now)
	if err != nil {
		return Repository{}, err
	}
	return Repository{ID: id, Name: strings.TrimSpace(name), URL: strings.TrimSpace(url), Realtime: false, CIIngestKey: ciKey, Releases: []string{}, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Service) Update(id, name, url string) (Repository, bool) {
	if s.storeFactory != nil {
		return s.updateStoreBacked(context.Background(), id, name, url)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.items[id]
	if !ok {
		return Repository{}, false
	}
	if strings.TrimSpace(name) != "" {
		r.Name = name
	}
	if strings.TrimSpace(url) != "" {
		r.URL = url
	}
	r.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.items[id] = r
	return r, true
}

func (s *Service) updateStoreBacked(ctx context.Context, id, name, url string) (Repository, bool) {
	repo, row, ok := s.loadStoreBacked(ctx, id)
	if !ok {
		return Repository{}, false
	}
	if strings.TrimSpace(name) != "" {
		repo.Name = strings.TrimSpace(name)
	}
	if strings.TrimSpace(url) != "" {
		repo.URL = strings.TrimSpace(url)
	}
	repo.UpdatedAt = persist.RFC3339Now()
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Repository{}, false
	}
	defer func() { _ = store.Close() }()
	_, err = store.Exec(ctx, "INSERT INTO sobs_apps (Id, Name, Slug, OwnerTeam, RepoUrl, DefaultEnvironment, Enabled, MetadataJson, IsDeleted, Version, CreatedAt, UpdatedAt) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, parseDateTime64BestEffort(?), parseDateTime64BestEffort(?))", id, repo.Name, row.slug, row.ownerTeam, repo.URL, row.defaultEnvironment, row.enabled, persist.JSONString(map[string]any{"realtime": repo.Realtime, "ci_ingest_key": repo.CIIngestKey}), 0, persist.Version(), repo.CreatedAt, repo.UpdatedAt)
	if err != nil {
		return Repository{}, false
	}
	repo.Releases = s.listReleasesForApp(ctx, id)
	return repo, true
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
	return true
}

func (s *Service) deleteStoreBacked(ctx context.Context, id string) bool {
	repo, row, ok := s.loadStoreBacked(ctx, id)
	if !ok {
		return false
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return false
	}
	defer func() { _ = store.Close() }()
	_, err = store.Exec(ctx, "INSERT INTO sobs_apps (Id, Name, Slug, OwnerTeam, RepoUrl, DefaultEnvironment, Enabled, MetadataJson, IsDeleted, Version, CreatedAt, UpdatedAt) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, parseDateTime64BestEffort(?), parseDateTime64BestEffort(?))", id, repo.Name, row.slug, row.ownerTeam, repo.URL, row.defaultEnvironment, row.enabled, persist.JSONString(map[string]any{"realtime": repo.Realtime, "ci_ingest_key": repo.CIIngestKey}), 1, persist.Version(), repo.CreatedAt, persist.RFC3339Now())
	return err == nil
}

func (s *Service) SetRealtime(id string, enabled bool) (Repository, bool) {
	if s.storeFactory != nil {
		repo, ok := s.updateMetadataStoreBacked(context.Background(), id, func(repo *Repository) {
			repo.Realtime = enabled
		})
		return repo, ok
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.items[id]
	if !ok {
		return Repository{}, false
	}
	r.Realtime = enabled
	r.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.items[id] = r
	return r, true
}

func (s *Service) RotateCIIngestKey(id string) (Repository, bool) {
	if s.storeFactory != nil {
		repo, ok := s.updateMetadataStoreBacked(context.Background(), id, func(repo *Repository) {
			repo.CIIngestKey = newSecret()
		})
		return repo, ok
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.items[id]
	if !ok {
		return Repository{}, false
	}
	r.CIIngestKey = newSecret()
	r.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.items[id] = r
	return r, true
}

func (s *Service) RevokeCIIngestKey(id string) (Repository, bool) {
	if s.storeFactory != nil {
		repo, ok := s.updateMetadataStoreBacked(context.Background(), id, func(repo *Repository) {
			repo.CIIngestKey = ""
		})
		return repo, ok
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.items[id]
	if !ok {
		return Repository{}, false
	}
	r.CIIngestKey = ""
	r.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.items[id] = r
	return r, true
}

func (s *Service) AddRelease(id, release string) (Repository, bool) {
	if s.storeFactory != nil {
		return s.addReleaseStoreBacked(context.Background(), id, release)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.items[id]
	if !ok {
		return Repository{}, false
	}
	if strings.TrimSpace(release) != "" {
		r.Releases = append(r.Releases, release)
		r.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		s.items[id] = r
	}
	return r, true
}

func (s *Service) addReleaseStoreBacked(ctx context.Context, id, release string) (Repository, bool) {
	repo, _, ok := s.loadStoreBacked(ctx, id)
	if !ok {
		return Repository{}, false
	}
	release = strings.TrimSpace(release)
	if release != "" {
		store, err := persist.Open(ctx, s.storeFactory)
		if err != nil {
			return Repository{}, false
		}
		defer func() { _ = store.Close() }()
		_, err = store.Exec(ctx, "INSERT INTO sobs_app_releases (Id, AppId, ReleaseVersion, CommitSha, BuildId, Environment, ReleasedAt, MetadataJson, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, parseDateTime64BestEffort(?), ?, ?, ?)", persist.NewID(), id, release, "", "", "default", persist.RFC3339Now(), "{}", 0, persist.Version())
		if err != nil {
			return Repository{}, false
		}
	}
	repo.Releases = s.listReleasesForApp(ctx, id)
	repo.UpdatedAt = persist.RFC3339Now()
	return repo, true
}

type storedAppRow struct {
	slug               string
	ownerTeam          string
	defaultEnvironment string
	enabled            uint8
}

func (s *Service) loadStoreBacked(ctx context.Context, id string) (Repository, storedAppRow, bool) {
	if err := s.ensureSchema(ctx); err != nil {
		return Repository{}, storedAppRow{}, false
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Repository{}, storedAppRow{}, false
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT Name, Slug, OwnerTeam, RepoUrl, DefaultEnvironment, Enabled, MetadataJson, CreatedAt, UpdatedAt FROM sobs_apps FINAL WHERE IsDeleted = 0 AND Id = ? LIMIT 1", id)
	if err != nil {
		return Repository{}, storedAppRow{}, false
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return Repository{}, storedAppRow{}, false
	}
	var repo Repository
	var row storedAppRow
	var metadataJSON string
	if err := rows.Scan(&repo.Name, &row.slug, &row.ownerTeam, &repo.URL, &row.defaultEnvironment, &row.enabled, &metadataJSON, &repo.CreatedAt, &repo.UpdatedAt); err != nil {
		return Repository{}, storedAppRow{}, false
	}
	repo.ID = id
	meta := persist.ParseJSONMap(metadataJSON)
	repo.Realtime = asBool(meta["realtime"])
	repo.CIIngestKey, _ = meta["ci_ingest_key"].(string)
	repo.Releases = s.listReleasesForApp(ctx, id)
	return repo, row, true
}

func (s *Service) updateMetadataStoreBacked(ctx context.Context, id string, mutate func(*Repository)) (Repository, bool) {
	repo, row, ok := s.loadStoreBacked(ctx, id)
	if !ok {
		return Repository{}, false
	}
	mutate(&repo)
	repo.UpdatedAt = persist.RFC3339Now()
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Repository{}, false
	}
	defer func() { _ = store.Close() }()
	_, err = store.Exec(ctx, "INSERT INTO sobs_apps (Id, Name, Slug, OwnerTeam, RepoUrl, DefaultEnvironment, Enabled, MetadataJson, IsDeleted, Version, CreatedAt, UpdatedAt) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, parseDateTime64BestEffort(?), parseDateTime64BestEffort(?))", id, repo.Name, row.slug, row.ownerTeam, repo.URL, row.defaultEnvironment, row.enabled, persist.JSONString(map[string]any{"realtime": repo.Realtime, "ci_ingest_key": repo.CIIngestKey}), 0, persist.Version(), repo.CreatedAt, repo.UpdatedAt)
	if err != nil {
		return Repository{}, false
	}
	repo.Releases = s.listReleasesForApp(ctx, id)
	return repo, true
}

func (s *Service) listReleasesForApp(ctx context.Context, appID string) []string {
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return nil
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT ReleaseVersion FROM sobs_app_releases FINAL WHERE AppId = ? AND IsDeleted = 0 ORDER BY ReleasedAt DESC", appID)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	out := []string{}
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return out
		}
		out = append(out, version)
	}
	return out
}

func asBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true") || strings.TrimSpace(typed) == "1"
	default:
		return false
	}
}

func ValidateGitHubToken(token string) bool {
	return len(strings.TrimSpace(token)) >= 12
}

func newSecret() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

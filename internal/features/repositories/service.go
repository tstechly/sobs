package repositories

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"sync"

	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/defaultstore"
	"github.com/abartrim/sobs/internal/features/persist"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/scrypt"
)

type Repository struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	URL             string   `json:"url"`
	Realtime        bool     `json:"realtime"`
	CIIngestKey     string   `json:"-"`
	CIIngestKeyHash string   `json:"-"`
	Releases        []string `json:"releases"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

const ciPushHashPrefix = "scrypt:v1:"

type Service struct {
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
		_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_apps (Id String, Name String, Slug String, OwnerTeam String, RepoUrl String, DefaultEnvironment String, Enabled UInt8 DEFAULT 1, MetadataJson String, IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0, CreatedAt DateTime64(3) DEFAULT now64(3), UpdatedAt DateTime64(3) DEFAULT now64(3)) ENGINE = ReplacingMergeTree(Version) ORDER BY (Slug, Id)")
		if err == nil {
			_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_app_releases (Id String, AppId String, ReleaseVersion String, CommitSha String, BuildId String, Environment String, ReleasedAt DateTime64(3) DEFAULT now64(3), MetadataJson String, IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version) ORDER BY (AppId, ReleaseVersion, Id)")
		}
		s.schemaErr = err
	})
	return s.schemaErr
}

func (s *Service) List() []Repository {
	return s.listStoreBacked(context.Background())
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
		item.CIIngestKeyHash = strings.TrimSpace(asString(meta["ci_ingest_key_hash"]))
		item.CIIngestKey = strings.TrimSpace(asString(meta["ci_ingest_key"]))
		item.Releases = releasesByApp[item.ID]
		out = append(out, item)
	}
	return out
}

func (s *Service) Create(name, url string) (Repository, error) {
	return s.createStoreBacked(context.Background(), name, url)
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
	metadata := persist.JSONString(map[string]any{"realtime": false, "ci_ingest_key_hash": ""})
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Repository{}, err
	}
	defer func() { _ = store.Close() }()
	_, err = store.Exec(ctx, "INSERT INTO sobs_apps (Id, Name, Slug, OwnerTeam, RepoUrl, DefaultEnvironment, Enabled, MetadataJson, IsDeleted, Version, CreatedAt, UpdatedAt) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, parseDateTime64BestEffort(?), parseDateTime64BestEffort(?))", id, strings.TrimSpace(name), id, "", strings.TrimSpace(url), "default", 1, metadata, 0, persist.Version(), now, now)
	if err != nil {
		return Repository{}, err
	}
	return Repository{ID: id, Name: strings.TrimSpace(name), URL: strings.TrimSpace(url), Realtime: false, CIIngestKeyHash: "", Releases: []string{}, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Service) Update(id, name, url string) (Repository, bool) {
	return s.updateStoreBacked(context.Background(), id, name, url)
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
	_, err = store.Exec(ctx, "INSERT INTO sobs_apps (Id, Name, Slug, OwnerTeam, RepoUrl, DefaultEnvironment, Enabled, MetadataJson, IsDeleted, Version, CreatedAt, UpdatedAt) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, parseDateTime64BestEffort(?), parseDateTime64BestEffort(?))", id, repo.Name, row.slug, row.ownerTeam, repo.URL, row.defaultEnvironment, row.enabled, persist.JSONString(map[string]any{"realtime": repo.Realtime, "ci_ingest_key_hash": repo.CIIngestKeyHash}), 0, persist.Version(), repo.CreatedAt, repo.UpdatedAt)
	if err != nil {
		return Repository{}, false
	}
	repo.Releases = s.listReleasesForApp(ctx, id)
	return repo, true
}

func (s *Service) Delete(id string) bool {
	return s.deleteStoreBacked(context.Background(), id)
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
	_, err = store.Exec(ctx, "INSERT INTO sobs_apps (Id, Name, Slug, OwnerTeam, RepoUrl, DefaultEnvironment, Enabled, MetadataJson, IsDeleted, Version, CreatedAt, UpdatedAt) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, parseDateTime64BestEffort(?), parseDateTime64BestEffort(?))", id, repo.Name, row.slug, row.ownerTeam, repo.URL, row.defaultEnvironment, row.enabled, persist.JSONString(map[string]any{"realtime": repo.Realtime, "ci_ingest_key_hash": repo.CIIngestKeyHash}), 1, persist.Version(), repo.CreatedAt, persist.RFC3339Now())
	return err == nil
}

func (s *Service) SetRealtime(id string, enabled bool) (Repository, bool) {
	repo, ok := s.updateMetadataStoreBacked(context.Background(), id, func(repo *Repository) {
		repo.Realtime = enabled
	})
	return repo, ok
}

func (s *Service) RotateCIIngestKey(id string) (Repository, string, bool) {
	plain := newCIIngestKey()
	repo, ok := s.updateMetadataStoreBacked(context.Background(), id, func(repo *Repository) {
		repo.CIIngestKeyHash = hashAPIKey(plain)
		repo.CIIngestKey = ""
	})
	if !ok {
		return Repository{}, "", false
	}
	return repo, plain, true
}

func (s *Service) RevokeCIIngestKey(id string) (Repository, bool) {
	repo, ok := s.updateMetadataStoreBacked(context.Background(), id, func(repo *Repository) {
		repo.CIIngestKey = ""
		repo.CIIngestKeyHash = ""
	})
	return repo, ok
}

func (s *Service) AddRelease(id, release string) (Repository, bool) {
	return s.addReleaseStoreBacked(context.Background(), id, release)
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
	repo.CIIngestKeyHash = strings.TrimSpace(asString(meta["ci_ingest_key_hash"]))
	repo.CIIngestKey = strings.TrimSpace(asString(meta["ci_ingest_key"]))
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
	_, err = store.Exec(ctx, "INSERT INTO sobs_apps (Id, Name, Slug, OwnerTeam, RepoUrl, DefaultEnvironment, Enabled, MetadataJson, IsDeleted, Version, CreatedAt, UpdatedAt) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, parseDateTime64BestEffort(?), parseDateTime64BestEffort(?))", id, repo.Name, row.slug, row.ownerTeam, repo.URL, row.defaultEnvironment, row.enabled, persist.JSONString(map[string]any{"realtime": repo.Realtime, "ci_ingest_key_hash": repo.CIIngestKeyHash}), 0, persist.Version(), repo.CreatedAt, repo.UpdatedAt)
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

func asString(value any) string {
	if v, ok := value.(string); ok {
		return v
	}
	return ""
}

func ciPushHashKey() []byte {
	secret := strings.TrimSpace(os.Getenv("SOBS_SECRET_KEY"))
	if secret == "" {
		secret = "sobs-dev-secret-key"
	}
	key := blake2b.Sum256([]byte(secret + "|sobs-ci-hash-v1"))
	return key[:]
}

func hashAPIKey(value string) string {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return ""
	}
	digest, err := scrypt.Key([]byte(raw), ciPushHashKey(), 1024, 8, 1, 32)
	if err != nil {
		return ""
	}
	return ciPushHashPrefix + hex.EncodeToString(digest)
}

func VerifyCIIngestKey(candidate string, storedHash string, legacyPlain string) bool {
	candidate = strings.TrimSpace(candidate)
	storedHash = strings.TrimSpace(storedHash)
	legacyPlain = strings.TrimSpace(legacyPlain)
	if candidate == "" {
		return false
	}
	if strings.HasPrefix(storedHash, ciPushHashPrefix) {
		candidateHash := hashAPIKey(candidate)
		if candidateHash == "" {
			return false
		}
		return subtle.ConstantTimeCompare([]byte(candidateHash), []byte(storedHash)) == 1
	}
	if legacyPlain != "" {
		return subtle.ConstantTimeCompare([]byte(candidate), []byte(legacyPlain)) == 1
	}
	return false
}

func ValidateGitHubToken(token string) bool {
	return len(strings.TrimSpace(token)) >= 12
}

func newCIIngestKey() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return "sobs_ci_" + base64.RawURLEncoding.EncodeToString(b)
}

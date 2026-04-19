package apps

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

type App struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type Release struct {
	ID        string `json:"id"`
	AppID     string `json:"app_id"`
	Version   string `json:"version"`
	CreatedAt string `json:"created_at"`
}

type Artifact struct {
	ID             string         `json:"id"`
	ReleaseID      string         `json:"release_id"`
	ArtifactType   string         `json:"artifact_type"`
	Name           string         `json:"name"`
	ContentType    string         `json:"content_type"`
	Size           uint64         `json:"size"`
	StorageRef     string         `json:"storage_ref"`
	ChecksumSHA256 string         `json:"checksum_sha256"`
	Platform       string         `json:"platform"`
	Architecture   string         `json:"architecture"`
	Metadata       map[string]any `json:"metadata"`
	UploadedAt     string         `json:"uploaded_at"`
}

type Service struct {
	mu            sync.RWMutex
	apps          map[string]App
	releasesByApp map[string][]Release
	releaseByID   map[string]Release
	artifactsByRelease map[string][]Artifact
	nextAppID     int64
	nextReleaseID int64
	nextArtifactID int64
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
		if err == nil {
			_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_release_artifacts (Id String, ReleaseId String, ArtifactType String, Name String, ContentType String, Size UInt64 DEFAULT 0, StorageRef String, ChecksumSha256 String, Platform String, Architecture String, MetadataJson String, UploadedAt DateTime64(3) DEFAULT now64(3), IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version) ORDER BY (ReleaseId, ArtifactType, Name, Id)")
		}
		s.schemaErr = err
	})
	return s.schemaErr
}

func (s *Service) ListApps() []App {
	if s.storeFactory != nil {
		return s.listAppsStoreBacked(context.Background())
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]App, 0, len(s.apps))
	for _, a := range s.apps {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func (s *Service) listAppsStoreBacked(ctx context.Context) []App {
	if err := s.ensureSchema(ctx); err != nil {
		return nil
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return nil
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT Id, Name, CreatedAt, UpdatedAt FROM sobs_apps FINAL WHERE IsDeleted = 0 ORDER BY Name")
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	out := []App{}
	for rows.Next() {
		var app App
		if err := rows.Scan(&app.ID, &app.Name, &app.CreatedAt, &app.UpdatedAt); err != nil {
			return out
		}
		out = append(out, app)
	}
	return out
}

func (s *Service) CreateApp(name string) (App, error) {
	if s.storeFactory != nil {
		return s.createAppStoreBacked(context.Background(), name)
	}
	if name == "" {
		return App{}, errors.New("name is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	s.mu.Lock()
	defer s.mu.Unlock()
	id := strconv.FormatInt(s.nextAppID, 10)
	s.nextAppID++
	a := App{ID: id, Name: name, CreatedAt: now, UpdatedAt: now}
	s.apps[id] = a
	return a, nil
}

func (s *Service) createAppStoreBacked(ctx context.Context, name string) (App, error) {
	if name == "" {
		return App{}, errors.New("name is required")
	}
	if err := s.ensureSchema(ctx); err != nil {
		return App{}, err
	}
	id := persist.NewID()
	now := persist.RFC3339Now()
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return App{}, err
	}
	defer func() { _ = store.Close() }()
	_, err = store.Exec(ctx, "INSERT INTO sobs_apps (Id, Name, Slug, OwnerTeam, RepoUrl, DefaultEnvironment, Enabled, MetadataJson, IsDeleted, Version, CreatedAt, UpdatedAt) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, parseDateTime64BestEffort(?), parseDateTime64BestEffort(?))", id, name, id, "", "", "default", 1, "{}", 0, persist.Version(), now, now)
	if err != nil {
		return App{}, err
	}
	return App{ID: id, Name: name, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Service) GetApp(id string) (App, bool) {
	if s.storeFactory != nil {
		return s.getAppStoreBacked(context.Background(), id)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.apps[id]
	return a, ok
}

func (s *Service) getAppStoreBacked(ctx context.Context, id string) (App, bool) {
	if err := s.ensureSchema(ctx); err != nil {
		return App{}, false
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return App{}, false
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT Id, Name, CreatedAt, UpdatedAt FROM sobs_apps FINAL WHERE IsDeleted = 0 AND Id = ? LIMIT 1", id)
	if err != nil {
		return App{}, false
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return App{}, false
	}
	var app App
	if err := rows.Scan(&app.ID, &app.Name, &app.CreatedAt, &app.UpdatedAt); err != nil {
		return App{}, false
	}
	return app, true
}

func (s *Service) PatchApp(id string, name *string) (App, error) {
	if s.storeFactory != nil {
		return s.patchAppStoreBacked(context.Background(), id, name)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.apps[id]
	if !ok {
		return App{}, errors.New("not found")
	}
	if name != nil {
		a.Name = *name
	}
	a.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	s.apps[id] = a
	return a, nil
}

func (s *Service) patchAppStoreBacked(ctx context.Context, id string, name *string) (App, error) {
	if err := s.ensureSchema(ctx); err != nil {
		return App{}, err
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return App{}, err
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT Name, Slug, OwnerTeam, RepoUrl, DefaultEnvironment, Enabled, MetadataJson, CreatedAt FROM sobs_apps FINAL WHERE IsDeleted = 0 AND Id = ? LIMIT 1", id)
	if err != nil {
		return App{}, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return App{}, errors.New("not found")
	}
	var currentName, slug, ownerTeam, repoURL, defaultEnvironment, metadataJSON, createdAt string
	var enabled uint8
	if err := rows.Scan(&currentName, &slug, &ownerTeam, &repoURL, &defaultEnvironment, &enabled, &metadataJSON, &createdAt); err != nil {
		return App{}, err
	}
	if name != nil {
		currentName = *name
	}
	updatedAt := persist.RFC3339Now()
	_, err = store.Exec(ctx, "INSERT INTO sobs_apps (Id, Name, Slug, OwnerTeam, RepoUrl, DefaultEnvironment, Enabled, MetadataJson, IsDeleted, Version, CreatedAt, UpdatedAt) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, parseDateTime64BestEffort(?), parseDateTime64BestEffort(?))", id, currentName, slug, ownerTeam, repoURL, defaultEnvironment, enabled, metadataJSON, 0, persist.Version(), createdAt, updatedAt)
	if err != nil {
		return App{}, err
	}
	return App{ID: id, Name: currentName, CreatedAt: createdAt, UpdatedAt: updatedAt}, nil
}

func (s *Service) ListReleases(appID string) []Release {
	if s.storeFactory != nil {
		return s.listReleasesStoreBacked(context.Background(), appID)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	in := s.releasesByApp[appID]
	out := make([]Release, len(in))
	copy(out, in)
	return out
}

func (s *Service) listReleasesStoreBacked(ctx context.Context, appID string) []Release {
	if err := s.ensureSchema(ctx); err != nil {
		return nil
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return nil
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT Id, AppId, ReleaseVersion, ReleasedAt FROM sobs_app_releases FINAL WHERE IsDeleted = 0 AND AppId = ? ORDER BY ReleasedAt DESC", appID)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	out := []Release{}
	for rows.Next() {
		var item Release
		if err := rows.Scan(&item.ID, &item.AppID, &item.Version, &item.CreatedAt); err != nil {
			return out
		}
		out = append(out, item)
	}
	return out
}

func (s *Service) CreateRelease(appID string, version string) (Release, error) {
	if s.storeFactory != nil {
		return s.createReleaseStoreBacked(context.Background(), appID, version)
	}
	if version == "" {
		return Release{}, errors.New("version is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.apps[appID]; !ok {
		return Release{}, errors.New("app not found")
	}
	id := strconv.FormatInt(s.nextReleaseID, 10)
	s.nextReleaseID++
	r := Release{ID: id, AppID: appID, Version: version, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	s.releasesByApp[appID] = append(s.releasesByApp[appID], r)
	s.releaseByID[id] = r
	return r, nil
}

func (s *Service) createReleaseStoreBacked(ctx context.Context, appID string, version string) (Release, error) {
	if version == "" {
		return Release{}, errors.New("version is required")
	}
	if _, ok := s.getAppStoreBacked(ctx, appID); !ok {
		return Release{}, errors.New("app not found")
	}
	if err := s.ensureSchema(ctx); err != nil {
		return Release{}, err
	}
	id := persist.NewID()
	createdAt := persist.RFC3339Now()
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Release{}, err
	}
	defer func() { _ = store.Close() }()
	_, err = store.Exec(ctx, "INSERT INTO sobs_app_releases (Id, AppId, ReleaseVersion, CommitSha, BuildId, Environment, ReleasedAt, MetadataJson, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, parseDateTime64BestEffort(?), ?, ?, ?)", id, appID, version, "", "", "default", createdAt, "{}", 0, persist.Version())
	if err != nil {
		return Release{}, err
	}
	return Release{ID: id, AppID: appID, Version: version, CreatedAt: createdAt}, nil
}

func (s *Service) GetRelease(releaseID string) (Release, bool) {
	if s.storeFactory != nil {
		return s.getReleaseStoreBacked(context.Background(), releaseID)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.releaseByID[releaseID]
	return r, ok
}

func (s *Service) getReleaseStoreBacked(ctx context.Context, releaseID string) (Release, bool) {
	if err := s.ensureSchema(ctx); err != nil {
		return Release{}, false
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Release{}, false
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT Id, AppId, ReleaseVersion, ReleasedAt FROM sobs_app_releases FINAL WHERE IsDeleted = 0 AND Id = ? LIMIT 1", releaseID)
	if err != nil {
		return Release{}, false
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return Release{}, false
	}
	var item Release
	if err := rows.Scan(&item.ID, &item.AppID, &item.Version, &item.CreatedAt); err != nil {
		return Release{}, false
	}
	return item, true
}

func (s *Service) ListArtifacts(releaseID string) []Artifact {
	if s.storeFactory != nil {
		return s.listArtifactsStoreBacked(context.Background(), releaseID)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	in := s.artifactsByRelease[releaseID]
	out := make([]Artifact, len(in))
	copy(out, in)
	return out
}

func (s *Service) listArtifactsStoreBacked(ctx context.Context, releaseID string) []Artifact {
	if err := s.ensureSchema(ctx); err != nil {
		return nil
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return nil
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT Id, ReleaseId, ArtifactType, Name, ContentType, Size, StorageRef, ChecksumSha256, Platform, Architecture, MetadataJson, UploadedAt FROM sobs_release_artifacts FINAL WHERE IsDeleted = 0 AND ReleaseId = ? ORDER BY UploadedAt DESC", releaseID)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	out := []Artifact{}
	for rows.Next() {
		var item Artifact
		var metadataJSON string
		if err := rows.Scan(&item.ID, &item.ReleaseID, &item.ArtifactType, &item.Name, &item.ContentType, &item.Size, &item.StorageRef, &item.ChecksumSHA256, &item.Platform, &item.Architecture, &metadataJSON, &item.UploadedAt); err != nil {
			return out
		}
		item.Metadata = persist.ParseJSONMap(metadataJSON)
		out = append(out, item)
	}
	return out
}

func (s *Service) CreateArtifact(releaseID, artifactType, name, contentType string, size uint64, storageRef, checksumSHA256, platform, architecture string, metadata map[string]any) (Artifact, error) {
	if s.storeFactory != nil {
		return s.createArtifactStoreBacked(context.Background(), releaseID, artifactType, name, contentType, size, storageRef, checksumSHA256, platform, architecture, metadata)
	}
	if strings.TrimSpace(releaseID) == "" || strings.TrimSpace(artifactType) == "" || strings.TrimSpace(name) == "" {
		return Artifact{}, errors.New("release_id, artifact_type, and name are required")
	}
	if _, ok := s.releaseByID[releaseID]; !ok {
		return Artifact{}, errors.New("release not found")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := strconv.FormatInt(s.nextArtifactID, 10)
	s.nextArtifactID++
	item := Artifact{
		ID:             id,
		ReleaseID:      releaseID,
		ArtifactType:   strings.TrimSpace(artifactType),
		Name:           strings.TrimSpace(name),
		ContentType:    strings.TrimSpace(contentType),
		Size:           size,
		StorageRef:     strings.TrimSpace(storageRef),
		ChecksumSHA256: strings.TrimSpace(checksumSHA256),
		Platform:       strings.TrimSpace(platform),
		Architecture:   strings.TrimSpace(architecture),
		Metadata:       metadata,
		UploadedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	s.artifactsByRelease[releaseID] = append(s.artifactsByRelease[releaseID], item)
	return item, nil
}

func (s *Service) createArtifactStoreBacked(ctx context.Context, releaseID, artifactType, name, contentType string, size uint64, storageRef, checksumSHA256, platform, architecture string, metadata map[string]any) (Artifact, error) {
	if strings.TrimSpace(releaseID) == "" || strings.TrimSpace(artifactType) == "" || strings.TrimSpace(name) == "" {
		return Artifact{}, errors.New("release_id, artifact_type, and name are required")
	}
	if _, ok := s.getReleaseStoreBacked(ctx, releaseID); !ok {
		return Artifact{}, errors.New("release not found")
	}
	if err := s.ensureSchema(ctx); err != nil {
		return Artifact{}, err
	}
	item := Artifact{
		ID:             persist.NewID(),
		ReleaseID:      strings.TrimSpace(releaseID),
		ArtifactType:   strings.TrimSpace(artifactType),
		Name:           strings.TrimSpace(name),
		ContentType:    strings.TrimSpace(contentType),
		Size:           size,
		StorageRef:     strings.TrimSpace(storageRef),
		ChecksumSHA256: strings.TrimSpace(checksumSHA256),
		Platform:       strings.TrimSpace(platform),
		Architecture:   strings.TrimSpace(architecture),
		Metadata:       metadata,
		UploadedAt:     persist.RFC3339Now(),
	}
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Artifact{}, err
	}
	defer func() { _ = store.Close() }()
	_, err = store.Exec(ctx, "INSERT INTO sobs_release_artifacts (Id, ReleaseId, ArtifactType, Name, ContentType, Size, StorageRef, ChecksumSha256, Platform, Architecture, MetadataJson, UploadedAt, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, parseDateTime64BestEffort(?), ?, ?)", item.ID, item.ReleaseID, item.ArtifactType, item.Name, item.ContentType, item.Size, item.StorageRef, item.ChecksumSHA256, item.Platform, item.Architecture, persist.JSONString(item.Metadata), item.UploadedAt, 0, persist.Version())
	if err != nil {
		return Artifact{}, err
	}
	return item, nil
}

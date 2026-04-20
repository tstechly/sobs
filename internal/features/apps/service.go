package apps

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/defaultstore"
	"github.com/abartrim/sobs/internal/features/persist"
)

var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

var (
	ErrNameRequired            = errors.New("name is required")
	ErrVersionRequired         = errors.New("version is required")
	ErrAppNotFound             = errors.New("app not found")
	ErrReleaseNotFound         = errors.New("release not found")
	ErrArtifactFieldsRequired  = errors.New("artifactType and name are required")
	ErrAppSlugAlreadyExists    = errors.New("app slug already exists")
	ErrNotFound                = errors.New("not found")
)

type App struct {
	ID                 string         `json:"id"`
	Name               string         `json:"name"`
	Slug               string         `json:"slug"`
	OwnerTeam          string         `json:"ownerTeam"`
	RepoURL            string         `json:"repoUrl"`
	DefaultEnvironment string         `json:"defaultEnvironment"`
	Enabled            bool           `json:"enabled"`
	Metadata           map[string]any `json:"metadata"`
	CreatedAt          string         `json:"createdAt"`
	UpdatedAt          string         `json:"updatedAt"`
}

type Release struct {
	ID          string         `json:"id"`
	AppID       string         `json:"appId"`
	Version     string         `json:"version"`
	CommitSHA   string         `json:"commitSha"`
	BuildID     string         `json:"buildId"`
	Environment string         `json:"environment"`
	ReleasedAt  string         `json:"releasedAt"`
	Metadata    map[string]any `json:"metadata"`
}

type Artifact struct {
	ID             string         `json:"id"`
	ReleaseID      string         `json:"releaseId"`
	ArtifactType   string         `json:"artifactType"`
	Name           string         `json:"name"`
	ContentType    string         `json:"contentType"`
	Size           uint64         `json:"size"`
	StorageRef     string         `json:"storageRef"`
	ChecksumSHA256 string         `json:"checksumSha256"`
	Platform       string         `json:"platform"`
	Architecture   string         `json:"architecture"`
	Metadata       map[string]any `json:"metadata"`
	UploadedAt     string         `json:"uploadedAt"`
}

type CreateAppInput struct {
	ID                 string
	Name               string
	Slug               string
	OwnerTeam          string
	RepoURL            string
	DefaultEnvironment string
	Enabled            *bool
	Metadata           map[string]any
}

type PatchAppInput struct {
	Name               *string
	Slug               *string
	OwnerTeam          *string
	RepoURL            *string
	DefaultEnvironment *string
	Enabled            *bool
	Metadata           map[string]any
	HasMetadata        bool
}

type CreateReleaseInput struct {
	ID          string
	Version     string
	CommitSHA   string
	BuildID     string
	Environment string
	ReleasedAt  string
	Metadata    map[string]any
}

type CreateArtifactInput struct {
	ID             string
	ArtifactType   string
	Name           string
	ContentType    string
	Size           uint64
	StorageRef     string
	ChecksumSHA256 string
	Platform       string
	Architecture   string
	Metadata       map[string]any
	UploadedAt     string
}

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
		if err == nil {
			_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_release_artifacts (Id String, ReleaseId String, ArtifactType String, Name String, ContentType String, Size UInt64 DEFAULT 0, StorageRef String, ChecksumSha256 String, Platform String, Architecture String, MetadataJson String, UploadedAt DateTime64(3) DEFAULT now64(3), IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version) ORDER BY (ReleaseId, ArtifactType, Name, Id)")
		}
		s.schemaErr = err
	})
	return s.schemaErr
}

func (s *Service) ListApps() []App {
	return s.listAppsStoreBacked(context.Background())
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
	rows, err := store.Query(ctx, "SELECT Id, Name, Slug, OwnerTeam, RepoUrl, DefaultEnvironment, Enabled, MetadataJson, CreatedAt, UpdatedAt FROM sobs_apps FINAL WHERE IsDeleted = 0 ORDER BY Name ASC")
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	out := make([]App, 0)
	for rows.Next() {
		app, scanErr := scanApp(rows)
		if scanErr != nil {
			return out
		}
		out = append(out, app)
	}
	return out
}

func (s *Service) CreateApp(input CreateAppInput) (App, error) {
	return s.createAppStoreBacked(context.Background(), input)
}

func (s *Service) createAppStoreBacked(ctx context.Context, input CreateAppInput) (App, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return App{}, ErrNameRequired
	}
	if err := s.ensureSchema(ctx); err != nil {
		return App{}, err
	}
	slug := appSlug(firstNonEmpty(strings.TrimSpace(input.Slug), name), name)
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return App{}, err
	}
	defer func() { _ = store.Close() }()
	if exists, err := appSlugExists(ctx, store, slug, ""); err != nil {
		return App{}, err
	} else if exists {
		return App{}, ErrAppSlugAlreadyExists
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	metadata := cloneMetadata(input.Metadata)
	createdAt := nowISO()
	updatedAt := nowISO()
	app := App{
		ID:                 firstNonEmpty(strings.TrimSpace(input.ID), persist.NewID()),
		Name:               name,
		Slug:               slug,
		OwnerTeam:          strings.TrimSpace(input.OwnerTeam),
		RepoURL:            strings.TrimSpace(input.RepoURL),
		DefaultEnvironment: strings.TrimSpace(input.DefaultEnvironment),
		Enabled:            enabled,
		Metadata:           metadata,
		CreatedAt:          createdAt,
		UpdatedAt:          updatedAt,
	}
	if app.DefaultEnvironment == "" {
		app.DefaultEnvironment = ""
	}
	_, err = store.Exec(ctx, "INSERT INTO sobs_apps (Id, Name, Slug, OwnerTeam, RepoUrl, DefaultEnvironment, Enabled, MetadataJson, IsDeleted, Version, CreatedAt, UpdatedAt) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, parseDateTime64BestEffort(?), parseDateTime64BestEffort(?))", app.ID, app.Name, app.Slug, app.OwnerTeam, app.RepoURL, app.DefaultEnvironment, boolToUInt8(app.Enabled), persist.JSONString(app.Metadata), 0, versionMillis(), app.CreatedAt, app.UpdatedAt)
	if err != nil {
		return App{}, err
	}
	return app, nil
}

func (s *Service) GetApp(id string) (App, bool) {
	return s.getAppStoreBacked(context.Background(), id)
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
	rows, err := store.Query(ctx, "SELECT Id, Name, Slug, OwnerTeam, RepoUrl, DefaultEnvironment, Enabled, MetadataJson, CreatedAt, UpdatedAt FROM sobs_apps FINAL WHERE IsDeleted = 0 AND Id = ? LIMIT 1", id)
	if err != nil {
		return App{}, false
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return App{}, false
	}
	app, err := scanApp(rows)
	if err != nil {
		return App{}, false
	}
	return app, true
}

func (s *Service) PatchApp(id string, input PatchAppInput) (App, error) {
	return s.patchAppStoreBacked(context.Background(), id, input)
}

func (s *Service) patchAppStoreBacked(ctx context.Context, id string, input PatchAppInput) (App, error) {
	current, ok := s.getAppStoreBacked(ctx, id)
	if !ok {
		return App{}, ErrNotFound
	}
	if err := s.ensureSchema(ctx); err != nil {
		return App{}, err
	}
	updated := current
	if input.Name != nil {
		updated.Name = strings.TrimSpace(*input.Name)
	}
	if updated.Name == "" {
		return App{}, ErrNameRequired
	}
	if input.Slug != nil {
		updated.Slug = appSlug(strings.TrimSpace(*input.Slug), updated.Name)
		if updated.Slug == "" {
			updated.Slug = appSlug(updated.Name, updated.Name)
		}
	} else {
		updated.Slug = appSlug(updated.Slug, updated.Name)
	}
	if input.OwnerTeam != nil {
		updated.OwnerTeam = strings.TrimSpace(*input.OwnerTeam)
	}
	if input.RepoURL != nil {
		updated.RepoURL = strings.TrimSpace(*input.RepoURL)
	}
	if input.DefaultEnvironment != nil {
		updated.DefaultEnvironment = strings.TrimSpace(*input.DefaultEnvironment)
	}
	if input.Enabled != nil {
		updated.Enabled = *input.Enabled
	}
	if input.HasMetadata {
		updated.Metadata = cloneMetadata(input.Metadata)
	}
	updated.UpdatedAt = nowISO()
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return App{}, err
	}
	defer func() { _ = store.Close() }()
	if exists, err := appSlugExists(ctx, store, updated.Slug, id); err != nil {
		return App{}, err
	} else if exists {
		return App{}, ErrAppSlugAlreadyExists
	}
	_, err = store.Exec(ctx, "INSERT INTO sobs_apps (Id, Name, Slug, OwnerTeam, RepoUrl, DefaultEnvironment, Enabled, MetadataJson, IsDeleted, Version, CreatedAt, UpdatedAt) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, parseDateTime64BestEffort(?), parseDateTime64BestEffort(?))", updated.ID, updated.Name, updated.Slug, updated.OwnerTeam, updated.RepoURL, updated.DefaultEnvironment, boolToUInt8(updated.Enabled), persist.JSONString(updated.Metadata), 0, versionMillis(), updated.CreatedAt, updated.UpdatedAt)
	if err != nil {
		return App{}, err
	}
	return updated, nil
}

func (s *Service) ListReleases(appID string) []Release {
	return s.listReleasesStoreBacked(context.Background(), appID)
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
	rows, err := store.Query(ctx, "SELECT Id, AppId, ReleaseVersion, CommitSha, BuildId, Environment, ReleasedAt, MetadataJson FROM sobs_app_releases FINAL WHERE AppId = ? AND IsDeleted = 0 ORDER BY ReleasedAt DESC", appID)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	out := make([]Release, 0)
	for rows.Next() {
		release, scanErr := scanRelease(rows)
		if scanErr != nil {
			return out
		}
		out = append(out, release)
	}
	return out
}

func (s *Service) CreateRelease(appID string, input CreateReleaseInput) (Release, error) {
	return s.createReleaseStoreBacked(context.Background(), appID, input)
}

func (s *Service) createReleaseStoreBacked(ctx context.Context, appID string, input CreateReleaseInput) (Release, error) {
	if _, ok := s.getAppStoreBacked(ctx, appID); !ok {
		return Release{}, ErrAppNotFound
	}
	version := strings.TrimSpace(input.Version)
	if version == "" {
		return Release{}, ErrVersionRequired
	}
	if err := s.ensureSchema(ctx); err != nil {
		return Release{}, err
	}
	release := Release{
		ID:          firstNonEmpty(strings.TrimSpace(input.ID), persist.NewID()),
		AppID:       appID,
		Version:     version,
		CommitSHA:   strings.TrimSpace(input.CommitSHA),
		BuildID:     strings.TrimSpace(input.BuildID),
		Environment: strings.TrimSpace(input.Environment),
		ReleasedAt:  firstNonEmpty(strings.TrimSpace(input.ReleasedAt), nowISO()),
		Metadata:    cloneMetadata(input.Metadata),
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Release{}, err
	}
	defer func() { _ = store.Close() }()
	_, err = store.Exec(ctx, "INSERT INTO sobs_app_releases (Id, AppId, ReleaseVersion, CommitSha, BuildId, Environment, ReleasedAt, MetadataJson, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, parseDateTime64BestEffort(?), ?, ?, ?)", release.ID, release.AppID, release.Version, release.CommitSHA, release.BuildID, release.Environment, release.ReleasedAt, persist.JSONString(release.Metadata), 0, versionMillis())
	if err != nil {
		return Release{}, err
	}
	return release, nil
}

func (s *Service) GetRelease(releaseID string) (Release, bool) {
	return s.getReleaseStoreBacked(context.Background(), releaseID)
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
	rows, err := store.Query(ctx, "SELECT Id, AppId, ReleaseVersion, CommitSha, BuildId, Environment, ReleasedAt, MetadataJson FROM sobs_app_releases FINAL WHERE Id = ? AND IsDeleted = 0 LIMIT 1", releaseID)
	if err != nil {
		return Release{}, false
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return Release{}, false
	}
	release, err := scanRelease(rows)
	if err != nil {
		return Release{}, false
	}
	return release, true
}

func (s *Service) ListArtifacts(releaseID string) []Artifact {
	return s.listArtifactsStoreBacked(context.Background(), releaseID)
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
	rows, err := store.Query(ctx, "SELECT Id, ReleaseId, ArtifactType, Name, ContentType, Size, StorageRef, ChecksumSha256, Platform, Architecture, MetadataJson, UploadedAt FROM sobs_release_artifacts FINAL WHERE ReleaseId = ? AND IsDeleted = 0 ORDER BY UploadedAt DESC", releaseID)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	out := make([]Artifact, 0)
	for rows.Next() {
		artifact, scanErr := scanArtifact(rows)
		if scanErr != nil {
			return out
		}
		out = append(out, artifact)
	}
	return out
}

func (s *Service) CreateArtifact(releaseID string, input CreateArtifactInput) (Artifact, error) {
	return s.createArtifactStoreBacked(context.Background(), releaseID, input)
}

func (s *Service) createArtifactStoreBacked(ctx context.Context, releaseID string, input CreateArtifactInput) (Artifact, error) {
	if _, ok := s.getReleaseStoreBacked(ctx, releaseID); !ok {
		return Artifact{}, ErrReleaseNotFound
	}
	artifactType := strings.TrimSpace(input.ArtifactType)
	name := strings.TrimSpace(input.Name)
	if artifactType == "" || name == "" {
		return Artifact{}, ErrArtifactFieldsRequired
	}
	if err := s.ensureSchema(ctx); err != nil {
		return Artifact{}, err
	}
	artifact := Artifact{
		ID:             firstNonEmpty(strings.TrimSpace(input.ID), persist.NewID()),
		ReleaseID:      releaseID,
		ArtifactType:   artifactType,
		Name:           name,
		ContentType:    strings.TrimSpace(input.ContentType),
		Size:           input.Size,
		StorageRef:     strings.TrimSpace(input.StorageRef),
		ChecksumSHA256: strings.TrimSpace(input.ChecksumSHA256),
		Platform:       strings.TrimSpace(input.Platform),
		Architecture:   strings.TrimSpace(input.Architecture),
		Metadata:       cloneMetadata(input.Metadata),
		UploadedAt:     firstNonEmpty(strings.TrimSpace(input.UploadedAt), nowISO()),
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Artifact{}, err
	}
	defer func() { _ = store.Close() }()
	_, err = store.Exec(ctx, "INSERT INTO sobs_release_artifacts (Id, ReleaseId, ArtifactType, Name, ContentType, Size, StorageRef, ChecksumSha256, Platform, Architecture, MetadataJson, UploadedAt, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, parseDateTime64BestEffort(?), ?, ?)", artifact.ID, artifact.ReleaseID, artifact.ArtifactType, artifact.Name, artifact.ContentType, artifact.Size, artifact.StorageRef, artifact.ChecksumSHA256, artifact.Platform, artifact.Architecture, persist.JSONString(artifact.Metadata), artifact.UploadedAt, 0, versionMillis())
	if err != nil {
		return Artifact{}, err
	}
	return artifact, nil
}

func appSlug(value string, fallback string) string {
	slug := slugPattern.ReplaceAllString(strings.TrimSpace(strings.ToLower(value)), "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = slugPattern.ReplaceAllString(strings.TrimSpace(strings.ToLower(fallback)), "-")
		slug = strings.Trim(slug, "-")
	}
	if slug == "" {
		slug = "app"
	}
	if len(slug) > 80 {
		return slug[:80]
	}
	return slug
}

func scanApp(rows extensionpoints.RowIterator) (App, error) {
	var app App
	var enabled uint8
	var metadataJSON string
	err := rows.Scan(&app.ID, &app.Name, &app.Slug, &app.OwnerTeam, &app.RepoURL, &app.DefaultEnvironment, &enabled, &metadataJSON, &app.CreatedAt, &app.UpdatedAt)
	if err != nil {
		return App{}, err
	}
	app.Enabled = enabled != 0
	app.Metadata = persist.ParseJSONMap(metadataJSON)
	return app, nil
}

func scanRelease(rows extensionpoints.RowIterator) (Release, error) {
	var release Release
	var metadataJSON string
	err := rows.Scan(&release.ID, &release.AppID, &release.Version, &release.CommitSHA, &release.BuildID, &release.Environment, &release.ReleasedAt, &metadataJSON)
	if err != nil {
		return Release{}, err
	}
	release.Metadata = persist.ParseJSONMap(metadataJSON)
	return release, nil
}

func scanArtifact(rows extensionpoints.RowIterator) (Artifact, error) {
	var artifact Artifact
	var metadataJSON string
	err := rows.Scan(&artifact.ID, &artifact.ReleaseID, &artifact.ArtifactType, &artifact.Name, &artifact.ContentType, &artifact.Size, &artifact.StorageRef, &artifact.ChecksumSHA256, &artifact.Platform, &artifact.Architecture, &metadataJSON, &artifact.UploadedAt)
	if err != nil {
		return Artifact{}, err
	}
	artifact.Metadata = persist.ParseJSONMap(metadataJSON)
	return artifact, nil
}

func appSlugExists(ctx context.Context, store extensionpoints.ClickHouseStore, slug string, excludeID string) (bool, error) {
	query := "SELECT Id FROM sobs_apps FINAL WHERE Slug = ? AND IsDeleted = 0 LIMIT 1"
	args := []any{slug}
	if strings.TrimSpace(excludeID) != "" {
		query = "SELECT Id FROM sobs_apps FINAL WHERE Slug = ? AND IsDeleted = 0 AND Id != ? LIMIT 1"
		args = append(args, excludeID)
	}
	rows, err := store.Query(ctx, query, args...)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	return rows.Next(), nil
}

func boolToUInt8(value bool) uint8 {
	if value {
		return 1
	}
	return 0
}

func cloneMetadata(value map[string]any) map[string]any {
	if len(value) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func versionMillis() uint64 {
	return uint64(time.Now().UnixMilli())
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
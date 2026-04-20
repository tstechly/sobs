package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abartrim/sobs/internal/config"
	"github.com/abartrim/sobs/internal/store"
)

var (
	sharedWebTestStorePath string
	sharedWebTestStoreOnce sync.Once
	sharedWebTestStoreMu   sync.Mutex
)

func webTestStorePath() string {
	sharedWebTestStoreOnce.Do(func() {
		path, err := os.MkdirTemp("", "sobs-web-test-chdb-")
		if err != nil {
			panic(err)
		}
		sharedWebTestStorePath = path
	})
	return sharedWebTestStorePath
}

func resetWebTestStore() {
	sharedWebTestStoreMu.Lock()
	defer sharedWebTestStoreMu.Unlock()

	factory := store.NewChdbStoreFactory(webTestStorePath())
	clickhouse, err := factory.Open(context.Background())
	if err != nil {
		panic(err)
	}
	defer func() { _ = clickhouse.Close() }()

	rows, err := clickhouse.Query(context.Background(), "SELECT name FROM system.tables WHERE database = currentDatabase() ORDER BY name")
	if err != nil {
		return
	}
	defer func() { _ = rows.Close() }()

	tableNames := make([]string, 0)
	for rows.Next() {
		var name string
		if scanErr := rows.Scan(&name); scanErr == nil {
			name = strings.TrimSpace(name)
			if name != "" {
				tableNames = append(tableNames, name)
			}
		}
	}

	for _, name := range tableNames {
		if _, err := clickhouse.Exec(context.Background(), "DROP TABLE IF EXISTS "+sanitizeIdentifier(name)); err == nil {
			continue
		}
		_, _ = clickhouse.Exec(context.Background(), "DROP VIEW IF EXISTS "+sanitizeIdentifier(name))
	}
}

func newTestServer() *Server {
	resetWebTestStore()
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	return NewServer(cfg, store.NewChdbStoreFactory(webTestStorePath()))
}

func uniqueRegistryValue(prefix string) string {
	return prefix + "-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
}

func TestV1AppsParityCreateListGetPatch(t *testing.T) {
	srv := newTestServer()
	appName := uniqueRegistryValue("Checkout Web")
	appSlug := uniqueRegistryValue("checkout-web")

	createReq := httptest.NewRequest(http.MethodPost, "http://example.com/v1/apps", bytes.NewReader([]byte(`{"name":"`+appName+`","slug":"`+appSlug+`","ownerTeam":"frontend","repoUrl":"https://github.com/example/checkout","defaultEnvironment":"prod","metadata":{"tier":"critical"}}`)))
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}
	appID, _ := created["id"].(string)
	if appID == "" {
		t.Fatal("expected app id")
	}
	if created["slug"] != appSlug || created["ownerTeam"] != "frontend" {
		t.Fatalf("unexpected create payload: %#v", created)
	}
	metadata, _ := created["metadata"].(map[string]any)
	if metadata["tier"] != "critical" {
		t.Fatalf("expected metadata to round-trip, got %#v", metadata)
	}

	listReq := httptest.NewRequest(http.MethodGet, "http://example.com/v1/apps?q=checkout", nil)
	listRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listPayload []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("unmarshal list response: %v", err)
	}
	if len(listPayload) != 1 || listPayload[0]["slug"] != appSlug {
		t.Fatalf("unexpected list payload: %#v", listPayload)
	}

	getReq := httptest.NewRequest(http.MethodGet, "http://example.com/v1/apps/"+appID, nil)
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}

	patchReq := httptest.NewRequest(http.MethodPatch, "http://example.com/v1/apps/"+appID, bytes.NewReader([]byte(`{"name":"Checkout Web 2","enabled":false,"metadata":{"tier":"standard"}}`)))
	patchRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", patchRec.Code, patchRec.Body.String())
	}
	var patched map[string]any
	if err := json.Unmarshal(patchRec.Body.Bytes(), &patched); err != nil {
		t.Fatalf("unmarshal patch response: %v", err)
	}
	if patched["name"] != "Checkout Web 2" || patched["enabled"] != false {
		t.Fatalf("unexpected patch payload: %#v", patched)
	}
	patchedMeta, _ := patched["metadata"].(map[string]any)
	if patchedMeta["tier"] != "standard" {
		t.Fatalf("unexpected patched metadata: %#v", patchedMeta)
	}
}

func TestV1AppsParityConflictsAndValidation(t *testing.T) {
	srv := newTestServer()
	appSlug := uniqueRegistryValue("checkout-web")

	missingNameReq := httptest.NewRequest(http.MethodPost, "http://example.com/v1/apps", bytes.NewReader([]byte(`{}`)))
	missingNameRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(missingNameRec, missingNameReq)
	if missingNameRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", missingNameRec.Code, missingNameRec.Body.String())
	}

	firstReq := httptest.NewRequest(http.MethodPost, "http://example.com/v1/apps", bytes.NewReader([]byte(`{"name":"Checkout Web","slug":"`+appSlug+`"}`)))
	firstRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	dupeReq := httptest.NewRequest(http.MethodPost, "http://example.com/v1/apps", bytes.NewReader([]byte(`{"name":"Checkout API","slug":"`+appSlug+`"}`)))
	dupeRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(dupeRec, dupeReq)
	if dupeRec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", dupeRec.Code, dupeRec.Body.String())
	}
}

func TestV1ReleasesAndArtifactsParity(t *testing.T) {
	srv := newTestServer()
	appName := uniqueRegistryValue("Checkout Web")
	appSlug := uniqueRegistryValue("checkout-web")

	createAppReq := httptest.NewRequest(http.MethodPost, "http://example.com/v1/apps", bytes.NewReader([]byte(`{"name":"`+appName+`","slug":"`+appSlug+`"}`)))
	createAppRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createAppRec, createAppReq)
	if createAppRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", createAppRec.Code, createAppRec.Body.String())
	}
	var app map[string]any
	if err := json.Unmarshal(createAppRec.Body.Bytes(), &app); err != nil {
		t.Fatalf("unmarshal app response: %v", err)
	}
	appID := app["id"].(string)

	createReleaseReq := httptest.NewRequest(http.MethodPost, "http://example.com/v1/apps/"+appID+"/releases", bytes.NewReader([]byte(`{"version":"1.2.3","commitSha":"abc123def456","environment":"prod"}`)))
	createReleaseRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createReleaseRec, createReleaseReq)
	if createReleaseRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", createReleaseRec.Code, createReleaseRec.Body.String())
	}
	var release map[string]any
	if err := json.Unmarshal(createReleaseRec.Body.Bytes(), &release); err != nil {
		t.Fatalf("unmarshal release response: %v", err)
	}
	releaseID := release["id"].(string)
	if release["version"] != "1.2.3" || release["commitSha"] != "abc123def456" {
		t.Fatalf("unexpected release payload: %#v", release)
	}

	listReq := httptest.NewRequest(http.MethodGet, "http://example.com/v1/apps/"+appID+"/releases", nil)
	listRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var releases []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &releases); err != nil {
		t.Fatalf("unmarshal releases response: %v", err)
	}
	if len(releases) != 1 || releases[0]["id"] != releaseID {
		t.Fatalf("unexpected releases payload: %#v", releases)
	}

	metaReq := httptest.NewRequest(http.MethodPost, "http://example.com/v1/releases/"+releaseID+"/artifacts/meta", bytes.NewReader([]byte(`{"artifactType":"js_sourcemap","name":"app.min.js.map","contentType":"application/json","size":3210,"storageRef":"s3://symbols/checkout/1.2.3/app.min.js.map"}`)))
	metaRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(metaRec, metaReq)
	if metaRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", metaRec.Code, metaRec.Body.String())
	}

	getReleaseReq := httptest.NewRequest(http.MethodGet, "http://example.com/v1/releases/"+releaseID, nil)
	getReleaseRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getReleaseRec, getReleaseReq)
	if getReleaseRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", getReleaseRec.Code, getReleaseRec.Body.String())
	}
	var releasePayload map[string]any
	if err := json.Unmarshal(getReleaseRec.Body.Bytes(), &releasePayload); err != nil {
		t.Fatalf("unmarshal release get response: %v", err)
	}
	releaseBody, _ := releasePayload["release"].(map[string]any)
	artifacts, _ := releasePayload["artifacts"].([]any)
	if releaseBody["version"] != "1.2.3" || len(artifacts) != 1 {
		t.Fatalf("unexpected release get payload: %#v", releasePayload)
	}

	listArtifactsReq := httptest.NewRequest(http.MethodGet, "http://example.com/v1/releases/"+releaseID+"/artifacts", nil)
	listArtifactsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listArtifactsRec, listArtifactsReq)
	if listArtifactsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", listArtifactsRec.Code, listArtifactsRec.Body.String())
	}
	var artifactList []map[string]any
	if err := json.Unmarshal(listArtifactsRec.Body.Bytes(), &artifactList); err != nil {
		t.Fatalf("unmarshal artifacts response: %v", err)
	}
	if len(artifactList) != 1 || artifactList[0]["name"] != "app.min.js.map" {
		t.Fatalf("unexpected artifacts payload: %#v", artifactList)
	}
}

func TestV1ReleaseArtifactsMetaValidation(t *testing.T) {
	srv := newTestServer()
	appName := uniqueRegistryValue("Checkout Web")

	missingReleaseReq := httptest.NewRequest(http.MethodPost, "http://example.com/v1/releases/missing/artifacts/meta", bytes.NewReader([]byte(`{"artifactType":"js_sourcemap","name":"main.js.map"}`)))
	missingReleaseRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(missingReleaseRec, missingReleaseReq)
	if missingReleaseRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", missingReleaseRec.Code, missingReleaseRec.Body.String())
	}

	createAppReq := httptest.NewRequest(http.MethodPost, "http://example.com/v1/apps", bytes.NewReader([]byte(`{"name":"`+appName+`"}`)))
	createAppRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createAppRec, createAppReq)
	var app map[string]any
	_ = json.Unmarshal(createAppRec.Body.Bytes(), &app)
	appID, _ := app["id"].(string)

	createReleaseReq := httptest.NewRequest(http.MethodPost, "http://example.com/v1/apps/"+appID+"/releases", bytes.NewReader([]byte(`{"version":"1.2.3"}`)))
	createReleaseRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createReleaseRec, createReleaseReq)
	var release map[string]any
	_ = json.Unmarshal(createReleaseRec.Body.Bytes(), &release)
	releaseID, _ := release["id"].(string)

	invalidMetaReq := httptest.NewRequest(http.MethodPost, "http://example.com/v1/releases/"+releaseID+"/artifacts/meta", bytes.NewReader([]byte(`{"name":"main.js.map"}`)))
	invalidMetaRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(invalidMetaRec, invalidMetaReq)
	if invalidMetaRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", invalidMetaRec.Code, invalidMetaRec.Body.String())
	}

	postArtifactsReq := httptest.NewRequest(http.MethodPost, "http://example.com/v1/releases/"+releaseID+"/artifacts", bytes.NewReader([]byte(`{"artifactType":"js_sourcemap","name":"main.js.map"}`)))
	postArtifactsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(postArtifactsRec, postArtifactsReq)
	if postArtifactsRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%s", postArtifactsRec.Code, postArtifactsRec.Body.String())
	}
}
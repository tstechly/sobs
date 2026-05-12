package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

// TestV1Apps_Create tests POST /v1/apps - Create application
func TestV1Apps_Create(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	payload := map[string]interface{}{
		"name":        "test-app",
		"description": "Test application",
	}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(baseURL+"/v1/apps", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// 201 on first create, 409 if app already exists from prior runs, 200 on idempotent re-create.
	assertStatusIn(t, resp, "POST /v1/apps", http.StatusCreated, http.StatusOK, http.StatusConflict)
}

// TestV1Apps_Get tests GET /v1/apps/<app_id> - Get application details
func TestV1Apps_Get(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	resp, err := http.Get(baseURL + "/v1/apps/test-app-id")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	assertStatusIn(t, resp, "GET /v1/apps/test-app-id", http.StatusNotFound)
}

// TestV1Apps_Patch tests PATCH /v1/apps/<app_id> - Update application
func TestV1Apps_Patch(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	payload := map[string]interface{}{
		"description": "Updated description",
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("PATCH", baseURL+"/v1/apps/test-app-id", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	assertStatusIn(t, resp, "PATCH /v1/apps/test-app-id", http.StatusNotFound)
}

// TestV1Apps_ListReleases tests GET /v1/apps/<app_id>/releases - List releases
func TestV1Apps_ListReleases(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	resp, err := http.Get(baseURL + "/v1/apps/test-app-id/releases")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	assertStatusIn(t, resp, "GET /v1/apps/test-app-id/releases", http.StatusNotFound)
}

// TestV1Releases_Create tests POST /v1/apps/<app_id>/releases - Create release
func TestV1Releases_Create(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	payload := map[string]interface{}{
		"version": "1.0.0",
	}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(baseURL+"/v1/apps/test-app-id/releases", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	assertStatusIn(t, resp, "POST /v1/apps/test-app-id/releases", http.StatusNotFound)
}

// TestV1Releases_Get tests GET /v1/releases/<release_id> - Get release
func TestV1Releases_Get(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	resp, err := http.Get(baseURL + "/v1/releases/test-release-id")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	assertStatusIn(t, resp, "GET /v1/releases/test-release-id", http.StatusNotFound)
}

// TestV1Releases_ListArtifacts tests GET /v1/releases/<release_id>/artifacts - List artifacts
func TestV1Releases_ListArtifacts(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	resp, err := http.Get(baseURL + "/v1/releases/test-release-id/artifacts")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	assertStatusIn(t, resp, "GET /v1/releases/test-release-id/artifacts", http.StatusNotFound)
}

// TestV1Releases_UpdateArtifactMeta tests POST /v1/releases/<release_id>/artifacts/meta
func TestV1Releases_UpdateArtifactMeta(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	payload := map[string]interface{}{
		"version": "1.0.0",
	}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(baseURL+"/v1/releases/test-release-id/artifacts/meta", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	assertStatusIn(t, resp, "POST /v1/releases/test-release-id/artifacts/meta", http.StatusNotFound)
}

// TestErrors_Resolve tests POST /errors/<error_id>/resolve - Resolve error
func TestErrors_Resolve(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	resp, err := http.Post(baseURL+"/errors/test-error-id/resolve", "application/json", nil)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	assertStatusIn(t, resp, "POST /errors/test-error-id/resolve", http.StatusOK)
}

// TestRepositories_Create tests POST /settings/repositories - Add repository
func TestRepositories_Create(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	payload := map[string]interface{}{
		"name": "test-repo",
		"url":  "https://github.com/test/repo",
	}
	body, _ := json.Marshal(payload)

	resp, err := postJSONNoRedirect(baseURL+"/settings/repositories", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	assertStatusIn(t, resp, "POST /settings/repositories", http.StatusFound)
}

// TestRepositories_ValidateToken tests POST /settings/repositories/github-token/validate
func TestRepositories_ValidateToken(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	payload := map[string]interface{}{
		"token": "test-token",
	}
	body, _ := json.Marshal(payload)

	resp, err := postJSONNoRedirect(baseURL+"/settings/repositories/github-token/validate", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	assertStatusIn(t, resp, "POST /settings/repositories/github-token/validate", http.StatusFound)
}

// TestRepositories_Update tests POST /settings/repositories/<app_id> - Update repository
func TestRepositories_Update(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	payload := map[string]interface{}{
		"description": "Updated description",
	}
	body, _ := json.Marshal(payload)

	resp, err := postJSONNoRedirect(baseURL+"/settings/repositories/test-app-id", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	assertStatusIn(t, resp, "POST /settings/repositories/test-app-id", http.StatusFound)
}

// TestRepositories_ToggleRealtime tests POST /settings/repositories/<app_id>/realtime-mode
func TestRepositories_ToggleRealtime(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	resp, err := postJSONNoRedirect(baseURL+"/settings/repositories/test-app-id/realtime-mode", nil)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	assertStatusIn(t, resp, "POST /settings/repositories/test-app-id/realtime-mode", http.StatusFound)
}

// TestRepositories_RevokeCIKey tests POST /settings/repositories/<app_id>/ci-ingest-key/revoke
func TestRepositories_RevokeCIKey(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	resp, err := postJSONNoRedirect(baseURL+"/settings/repositories/test-app-id/ci-ingest-key/revoke", nil)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	assertStatusIn(t, resp, "POST /settings/repositories/test-app-id/ci-ingest-key/revoke", http.StatusFound)
}

// TestRepositories_RotateCIKey tests POST /settings/repositories/<app_id>/ci-ingest-key/rotate
func TestRepositories_RotateCIKey(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	resp, err := postJSONNoRedirect(baseURL+"/settings/repositories/test-app-id/ci-ingest-key/rotate", nil)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	assertStatusIn(t, resp, "POST /settings/repositories/test-app-id/ci-ingest-key/rotate", http.StatusFound)
}

// TestRepositories_Delete tests POST /settings/repositories/<app_id>/delete
func TestRepositories_Delete(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	resp, err := postJSONNoRedirect(baseURL+"/settings/repositories/test-app-id/delete", nil)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	assertStatusIn(t, resp, "POST /settings/repositories/test-app-id/delete", http.StatusFound)
}

// TestMCP_Tools tests GET /mcp/tools - List MCP tools
func TestMCP_Tools(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	resp, err := http.Get(baseURL + "/mcp/tools")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	assertStatusIn(t, resp, "GET /mcp/tools", http.StatusOK)
	assertJSONBody(t, resp, "GET /mcp/tools")
}

// TestMCP_Protocol tests POST /mcp - MCP protocol endpoint
func TestMCP_Protocol(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "tools/list",
		"id":      1,
	}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(baseURL+"/mcp", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	assertStatusIn(t, resp, "POST /mcp", http.StatusUnauthorized)
}

// TestRepositories_ViewDetails tests GET /api/settings/repositories/<app_id>
// Note: this endpoint is not listed in endpoints.txt but tests exercise it.
func TestRepositories_ViewDetails(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	resp, err := http.Get(baseURL + "/api/settings/repositories/test-app-id")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	assertStatusIn(t, resp, "GET /api/settings/repositories/test-app-id", http.StatusNotFound)
}

// TestTags_ConditionSuggestions tests GET /api/settings/tags/condition-suggestions
func TestTags_ConditionSuggestions(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	resp, err := http.Get(baseURL + "/api/settings/tags/condition-suggestions")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	assertStatusIn(t, resp, "GET /api/settings/tags/condition-suggestions", http.StatusOK)
	assertJSONBody(t, resp, "GET /api/settings/tags/condition-suggestions")
}

// TestAIHelper_ActionsManifest tests GET /api/ai/helper/actions/manifest
func TestAIHelper_ActionsManifest(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	resp, err := http.Get(baseURL + "/api/ai/helper/actions/manifest")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	assertStatusIn(t, resp, "GET /api/ai/helper/actions/manifest", http.StatusOK)
	assertJSONBody(t, resp, "GET /api/ai/helper/actions/manifest")
}

// TestMetricsRules_Delete tests DELETE /metrics/rules/<rule_id>
// Note: endpoints.txt documents POST /metrics/rules/<rule_id>/delete; this DELETE variant
// is not registered (server returns 404).
func TestMetricsRules_Delete(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	req, err := http.NewRequest("DELETE", baseURL+"/metrics/rules/test-rule-id", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	assertStatusIn(t, resp, "DELETE /metrics/rules/test-rule-id", http.StatusNotFound)
}

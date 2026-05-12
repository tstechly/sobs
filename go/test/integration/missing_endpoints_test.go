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

	t.Logf("POST /v1/apps returned status: %d", resp.StatusCode)
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

	t.Logf("GET /v1/apps/test-app-id returned status: %d", resp.StatusCode)
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

	t.Logf("PATCH /v1/apps/test-app-id returned status: %d", resp.StatusCode)
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

	t.Logf("GET /v1/apps/test-app-id/releases returned status: %d", resp.StatusCode)
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

	t.Logf("POST /v1/apps/test-app-id/releases returned status: %d", resp.StatusCode)
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

	t.Logf("GET /v1/releases/test-release-id returned status: %d", resp.StatusCode)
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

	t.Logf("GET /v1/releases/test-release-id/artifacts returned status: %d", resp.StatusCode)
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

	t.Logf("POST /v1/releases/test-release-id/artifacts/meta returned status: %d", resp.StatusCode)
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

	t.Logf("POST /errors/test-error-id/resolve returned status: %d", resp.StatusCode)
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

	resp, err := http.Post(baseURL+"/settings/repositories", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	t.Logf("POST /settings/repositories returned status: %d", resp.StatusCode)
}

// TestRepositories_ValidateToken tests POST /settings/repositories/github-token/validate
func TestRepositories_ValidateToken(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	payload := map[string]interface{}{
		"token": "test-token",
	}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(baseURL+"/settings/repositories/github-token/validate", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	t.Logf("POST /settings/repositories/github-token/validate returned status: %d", resp.StatusCode)
}

// TestRepositories_Update tests POST /settings/repositories/<app_id> - Update repository
func TestRepositories_Update(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	payload := map[string]interface{}{
		"description": "Updated description",
	}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(baseURL+"/settings/repositories/test-app-id", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	t.Logf("POST /settings/repositories/test-app-id returned status: %d", resp.StatusCode)
}

// TestRepositories_ToggleRealtime tests POST /settings/repositories/<app_id>/realtime-mode
func TestRepositories_ToggleRealtime(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	resp, err := http.Post(baseURL+"/settings/repositories/test-app-id/realtime-mode", "application/json", nil)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	t.Logf("POST /settings/repositories/test-app-id/realtime-mode returned status: %d", resp.StatusCode)
}

// TestRepositories_RevokeCIKey tests POST /settings/repositories/<app_id>/ci-ingest-key/revoke
func TestRepositories_RevokeCIKey(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	resp, err := http.Post(baseURL+"/settings/repositories/test-app-id/ci-ingest-key/revoke", "application/json", nil)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	t.Logf("POST /settings/repositories/test-app-id/ci-ingest-key/revoke returned status: %d", resp.StatusCode)
}

// TestRepositories_RotateCIKey tests POST /settings/repositories/<app_id>/ci-ingest-key/rotate
func TestRepositories_RotateCIKey(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	resp, err := http.Post(baseURL+"/settings/repositories/test-app-id/ci-ingest-key/rotate", "application/json", nil)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	t.Logf("POST /settings/repositories/test-app-id/ci-ingest-key/rotate returned status: %d", resp.StatusCode)
}

// TestRepositories_Delete tests POST /settings/repositories/<app_id>/delete
func TestRepositories_Delete(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	resp, err := http.Post(baseURL+"/settings/repositories/test-app-id/delete", "application/json", nil)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	t.Logf("POST /settings/repositories/test-app-id/delete returned status: %d", resp.StatusCode)
}

// TestMCP_Tools tests GET /api/mcp/tools - List MCP tools
func TestMCP_Tools(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	resp, err := http.Get(baseURL + "/mcp/tools")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	t.Logf("GET /mcp/tools returned status: %d", resp.StatusCode)
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

	t.Logf("POST /mcp returned status: %d", resp.StatusCode)
}

// TestRepositories_ViewDetails tests GET /api/settings/repositories/<app_id>
func TestRepositories_ViewDetails(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	resp, err := http.Get(baseURL + "/api/settings/repositories/test-app-id")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	t.Logf("GET /api/settings/repositories/test-app-id returned status: %d", resp.StatusCode)
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

	t.Logf("GET /api/settings/tags/condition-suggestions returned status: %d", resp.StatusCode)
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

	t.Logf("GET /api/ai/helper/actions/manifest returned status: %d", resp.StatusCode)
}

// TestMetricsRules_Delete tests DELETE /api/metrics/rules/<rule_id>
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

	t.Logf("DELETE /metrics/rules/test-rule-id returned status: %d", resp.StatusCode)
}
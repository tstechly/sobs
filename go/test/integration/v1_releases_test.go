// Package integration tests for /v1/releases endpoints
package integration

import (
    "encoding/json"
    "net/http"
    "testing"
)

const baseURL = getBaseURL()  // Implement getBaseURL() to handle test URL

func TestGetRelease(t *testing.T) {
    // First create a release
    CreateRelease(t)

    // Get release details
    resp, err := http.Get(getBaseURL() + "/v1/releases/test-release-id")
    if err != nil {
        t.Fatalf("Failed to get release: %v", err)
    }
    defer resp.Body.Close()

    // Expect 200 OK
    if resp.StatusCode != http.StatusOK {
        t.Errorf("Expected 200, got %d", resp.StatusCode)
    }
}

func TestListReleaseArtifacts(t *testing.T) {
    // Create release first
    CreateRelease(t)

    // List artifacts
    resp, err := http.Get(getBaseURL() + "/v1/releases/test-release-id/artifacts")
    if err != nil {
        t.Fatalf("Failed to list artifacts: %v", err)
    }
    defer resp.Body.Close()

    // Expect 200 OK
    if resp.StatusCode != http.StatusOK {
        t.Errorf("Expected 200, got %d", resp.StatusCode)
    }
}

func TestUpdateArtifactMeta(t *testing.T) {
    // Create release first
    CreateRelease(t)

    // Update metadata
    payload := map[string]interface{}{
        "description": "Updated artifact metadata",
    }
    body, _ := json.Marshal(payload)

    resp, err := http.Post(getBaseURL() + "/v1/releases/test-release-id/artifacts/meta", "application/json", bytes.NewBuffer(body))
    if err != nil {
        t.Fatalf("Failed to update metadata: %v", err)
    }
    defer resp.Body.Close()

    // Expect 200 OK
    if resp.StatusCode != http.StatusOK {
        t.Errorf("Expected 200, got %d", resp.StatusCode)
    }
}

// helper to create a release for other tests
func CreateRelease(t *testing.T) {
    // Implementation to create a test release
    // Would need to create an app first if not already created
}
// Package integration tests for /v1/apps endpoints
package integration

import (
    "encoding/json"
    "net/http"
    "testing"
)

const baseURL = getBaseURL()  // Implement getBaseURL() to handle test URL

func TestCreateApp(t *testing.T) {
    baseURL := getBaseURL()
    // Create test application payload
    payload := map[string]interface{}{
        "name": "Test App",
        "description": "Integration test app",
    }
    body, _ := json.Marshal(payload)

    resp, err := http.Post(baseURL + "/v1/apps", "application/json", bytes.NewBuffer(body))
    if err != nil {
        t.Fatalf("Failed to create app: %v", err)
    }
    defer resp.Body.Close()

    // Expect 201 Created
    if resp.StatusCode != http.StatusCreated {
        t.Errorf("Expected 201, got %d", resp.StatusCode)
    }
}

func TestGetAppDetails(t *testing.T) {
    // First create an app
    CreateApp(t)

    // Get details
    resp, err := http.Get(getBaseURL() + "/v1/apps/test-app-id")  // Use actual test ID
    if err != nil {
        t.Fatalf("Failed to get app details: %v", err)
    }
    defer resp.Body.Close()

    // Expect 200 OK
    if resp.StatusCode != http.StatusOK {
        t.Errorf("Expected 200, got %d", resp.StatusCode)
    }
}

func TestUpdateApp(t *testing.T) {
    // Create app first
    CreateApp(t)

    // Update payload
    payload := map[string]interface{}{
        "description": "Updated test app",
    }
    body, _ := json.Marshal(payload)

    resp, err := http.Patch(getBaseURL() + "/v1/apps/test-app-id", "application/json", bytes.NewBuffer(body))
    if err != nil {
        t.Fatalf("Failed to update app: %v", err)
    }
    defer resp.Body.Close()

    // Expect 200 OK
    if resp.StatusCode != http.StatusOK {
        t.Errorf("Expected 200, got %d", resp.StatusCode)
    }
}

// helper to create an app for other tests
func CreateApp(t *testing.T) {
    // Implementation to create a test app
    // This would need to mock or actually create an app for subsequent tests
}
// Package integration tests for /errors endpoints
package integration

import (
    "encoding/json"
    "net/http"
    "testing"
)

const baseURL = getBaseURL()  // Implement getBaseURL() to handle test URL

func TestResolveError(t *testing.T) {
    // First create an error
    CreateError(t)

    // Resolve error
    resp, err := http.Post(getBaseURL()+"/errors/test-error-id/resolve", "application/json", nil)
    if err != nil {
        t.Fatalf("Failed to resolve error: %v", err)
    }
    defer resp.Body.Close()

    // Expect 200 OK
    if resp.StatusCode != http.StatusOK {
        t.Errorf("Expected 200, got %d", resp.StatusCode)
    }
}

func TestCreateError(t *testing.T) {
    payload := map[string]interface{}{
        "errorId": "test-error-id",
        "message": "Test error message",
        "service": "test-service",
    }
    body, _ := json.Marshal(payload)

    resp, err := http.Post(getBaseURL()+"/v1/errors", "application/json", bytes.NewBuffer(body))
    if err != nil {
        t.Fatalf("Failed to create error: %v", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusCreated {
        t.Errorf("Expected 201, got %d", resp.StatusCode)
    }
}

// helper to create an error for other tests
func CreateError(t *testing.T) {
    // Would create a test error first
}
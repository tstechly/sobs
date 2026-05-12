package integration

import (
    "bytes"
    "encoding/json"
    "net/http"
    "testing"
)

const baseURL = getBaseURL() // Implement getBaseURL() to handle test URL
func getBaseURL() string {
	if url := os.Getenv("SOBS_TEST_URL"); url != "" {
		return url
	}
	return "http://localhost:44317"
}

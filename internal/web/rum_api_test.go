package web

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestRUMAssetsAndClientToken(t *testing.T) {
	t.Setenv("SOBS_RUM_ASSET_SIGNING_KEY", "test-signing-key")
	srv := newTestServer()

	body := []byte("asset")
	ts := fmt.Sprintf("%d", time.Now().Unix())
	bodyHash := sha256.Sum256(body)
	payload := stringsJoin([]string{
		"POST",
		"/v1/rum/assets",
		ts,
		fmt.Sprintf("%x", bodyHash[:]),
		"application/octet-stream",
		"asset",
		"asset",
	}, "\n")
	mac := hmac.New(sha256.New, []byte("test-signing-key"))
	_, _ = mac.Write([]byte(payload))
	sig := fmt.Sprintf("%x", mac.Sum(nil))

	createReq := httptest.NewRequest(http.MethodPost, "http://example.com/v1/rum/assets", bytes.NewReader(body))
	createReq.Header.Set("X-SOBS-Asset-Timestamp", ts)
	createReq.Header.Set("X-SOBS-Asset-Signature", sig)
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRec.Code)
	}
	var asset map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &asset); err != nil {
		t.Fatalf("unmarshal asset: %v", err)
	}
	id, _ := asset["id"].(string)
	if id == "" {
		t.Fatal("expected id")
	}

	getReq := httptest.NewRequest(http.MethodGet, "http://example.com/v1/rum/assets/"+id, nil)
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRec.Code)
	}

	// When SOBS_RUM_CLIENT_AUTH_MODE is unset (default "none"), returns
	// {"enabled":false,...} with 200 — matching Python's behavior.
	tokenReq := httptest.NewRequest(http.MethodPost, "http://example.com/v1/rum/client-token", nil)
	tokenRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(tokenRec, tokenReq)
	if tokenRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", tokenRec.Code)
	}
	var tokenResp map[string]any
	if err := json.Unmarshal(tokenRec.Body.Bytes(), &tokenResp); err != nil {
		t.Fatalf("unmarshal token resp: %v", err)
	}
	enabled, _ := tokenResp["enabled"].(bool)
	if enabled {
		t.Fatal("expected enabled=false when auth mode is none")
	}
}

func TestRUMAssetUploadRequiresSignatureHeaders(t *testing.T) {
	t.Setenv("SOBS_RUM_ASSET_SIGNING_KEY", "test-signing-key")
	srv := newTestServer()

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/rum/assets", bytes.NewReader([]byte("asset")))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestRUMSignedAssetUploadAndDownload(t *testing.T) {
	t.Setenv("SOBS_RUM_ASSET_SIGNING_KEY", "test-signing-key")
	srv := newTestServer()

	body := []byte("png-bytes")
	ts := fmt.Sprintf("%d", time.Now().Unix())
	bodyHash := sha256.Sum256(body)
	payload := stringsJoin([]string{
		"POST",
		"/v1/rum/assets",
		ts,
		fmt.Sprintf("%x", bodyHash[:]),
		"image/png",
		"screenshot",
		"shot.png",
	}, "\n")
	mac := hmac.New(sha256.New, []byte(os.Getenv("SOBS_RUM_ASSET_SIGNING_KEY")))
	_, _ = mac.Write([]byte(payload))
	sig := fmt.Sprintf("%x", mac.Sum(nil))

	createReq := httptest.NewRequest(http.MethodPost, "http://example.com/v1/rum/assets?type=screenshot&name=shot.png", bytes.NewReader(body))
	createReq.Header.Set("Content-Type", "image/png")
	createReq.Header.Set("X-SOBS-Asset-Timestamp", ts)
	createReq.Header.Set("X-SOBS-Asset-Signature", sig)
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}

	var upload map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &upload); err != nil {
		t.Fatalf("unmarshal upload: %v", err)
	}
	id, _ := upload["id"].(string)
	if id == "" {
		t.Fatal("expected id")
	}

	getReq := httptest.NewRequest(http.MethodGet, "http://example.com/v1/rum/assets/"+id, nil)
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRec.Code)
	}
	if ct := getRec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("expected image/png content-type, got %q", ct)
	}
	if !bytes.Equal(getRec.Body.Bytes(), body) {
		t.Fatalf("expected binary body match")
	}
}

func stringsJoin(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out += sep + parts[i]
	}
	return out
}

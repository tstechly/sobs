package web

import (
	"errors"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	rumfeature "github.com/abartrim/sobs/internal/features/rum"
)

var rumAssetIDRegex = regexp.MustCompile(`^[a-f0-9]{32}$`)

func (s *Server) v1RUMAssets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
		return
	}
	if len(body) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "asset body is required"})
		return
	}
	if len(body) > rumAssetMaxBytes() {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "asset exceeds max allowed size"})
		return
	}

	contentType := strings.TrimSpace(strings.SplitN(r.Header.Get("Content-Type"), ";", 2)[0])
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	assetType := strings.TrimSpace(r.URL.Query().Get("type"))
	if assetType == "" {
		assetType = "asset"
	}
	assetName := strings.TrimSpace(r.URL.Query().Get("name"))
	if assetName == "" {
		assetName = "asset"
	}

	ok, status, msg := verifyRUMAssetSignature(r, body, contentType, assetType, assetName)
	if !ok {
		writeJSON(w, status, map[string]string{"error": msg})
		return
	}

	meta, err := s.rumService.CreateUploadedAsset(assetType, assetName, contentType, body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":          meta.ID,
		"type":        meta.Type,
		"name":        meta.OriginalName,
		"contentType": meta.ContentType,
		"size":        meta.Size,
		"url":         "/v1/rum/assets/" + meta.ID,
	})
}

func (s *Server) v1RUMAssetByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/rum/assets/")
	if !rumAssetIDRegex.MatchString(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid asset id"})
		return
	}
	meta, body, err := s.rumService.GetUploadedAsset(id)
	if err == nil {
		w.Header().Set("Content-Type", meta.ContentType)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
		return
	}
	if errors.Is(err, rumfeature.ErrUploadedAssetMetadataUnavailable) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "asset metadata unavailable"})
		return
	}
	if errors.Is(err, rumfeature.ErrInvalidUploadedAssetMetadata) {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "invalid asset metadata"})
		return
	}
	if errors.Is(err, os.ErrNotExist) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "asset metadata unavailable"})
}

// v1RUMClientToken mirrors Python's issue_rum_client_token endpoint.
// When SOBS_RUM_CLIENT_AUTH_MODE is "none"/"off"/"disabled"/unset it returns
// {"enabled":false,"token":"","error":"RUM client auth is disabled"}.
// When configured it returns a signed token with origin/app/exp claims.
func (s *Server) v1RUMClientToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mode := s.rumService.AuthMode()
	if mode == "" || mode == "none" || mode == "off" || mode == "disabled" {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false, "token": "", "error": "RUM client auth is disabled"})
		return
	}

	if mode != "origin" && mode != "origin-session" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Invalid SOBS_RUM_CLIENT_AUTH_MODE"})
		return
	}

	signingKey := s.rumService.SigningKey()
	if signingKey == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "RUM client signing key is not configured"})
		return
	}

	var payload map[string]any
	_ = json.NewDecoder(r.Body).Decode(&payload)
	if payload == nil {
		payload = map[string]any{}
	}

	appName := strings.TrimSpace(stringVal(payload, "appName", stringVal(payload, "app", "")))

	origin := strings.TrimSpace(stringVal(payload, "origin", ""))
	if origin == "" {
		origin = requestOriginFromHeaders(r.Header.Get("Origin"), r.Header.Get("Referer"))
	}
	if origin == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "origin is required"})
		return
	}

	ttlSec := s.rumService.TokenTTL()
	if v, ok := payload["ttlSec"]; ok {
		switch n := v.(type) {
		case float64:
			ttlSec = int(n)
		}
	}
	if ttlSec < 30 {
		ttlSec = 30
	}
	if ttlSec > 86400 {
		ttlSec = 86400
	}

	token, exp := s.rumService.NewClientToken(signingKey, origin, appName, ttlSec)
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":   true,
		"token":     token,
		"expiresAt": exp,
		"origin":    origin,
		"app":       appName,
	})
}
func stringVal(m map[string]any, key, def string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return def
}

func verifyRUMAssetSignature(r *http.Request, body []byte, contentType, assetType, assetName string) (bool, int, string) {
	secret := strings.TrimSpace(os.Getenv("SOBS_RUM_ASSET_SIGNING_KEY"))
	if secret == "" {
		return false, http.StatusServiceUnavailable, "Asset upload signing key is not configured"
	}

	tsRaw := strings.TrimSpace(r.Header.Get("X-SOBS-Asset-Timestamp"))
	sig := strings.ToLower(strings.TrimSpace(r.Header.Get("X-SOBS-Asset-Signature")))
	if tsRaw == "" || sig == "" {
		return false, http.StatusUnauthorized, "Missing asset signature headers"
	}

	ts, err := strconv.ParseInt(tsRaw, 10, 64)
	if err != nil {
		return false, http.StatusUnauthorized, "Invalid asset signature timestamp"
	}

	window := int64(300)
	if raw := strings.TrimSpace(os.Getenv("SOBS_RUM_ASSET_SIGN_WINDOW_SEC")); raw != "" {
		if parsed, convErr := strconv.ParseInt(raw, 10, 64); convErr == nil && parsed > 0 {
			window = parsed
		}
	}
	if absInt64(time.Now().Unix()-ts) > window {
		return false, http.StatusUnauthorized, "Asset signature timestamp outside allowed window"
	}

	bodyHash := sha256.Sum256(body)
	payload := strings.Join([]string{
		strings.ToUpper(r.Method),
		r.URL.Path,
		tsRaw,
		fmt.Sprintf("%x", bodyHash[:]),
		strings.ToLower(strings.TrimSpace(contentType)),
		strings.ToLower(strings.TrimSpace(assetType)),
		assetName,
	}, "\n")

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	expected := fmt.Sprintf("%x", mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return false, http.StatusUnauthorized, "Invalid asset signature"
	}

	return true, http.StatusOK, ""
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func rumAssetMaxBytes() int {
	const defaultMax = 8 * 1024 * 1024
	raw := strings.TrimSpace(os.Getenv("SOBS_RUM_ASSET_MAX_BYTES"))
	if raw == "" {
		return defaultMax
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return defaultMax
	}
	if v < 1024 {
		return 1024
	}
	return v
}

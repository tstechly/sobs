package rum

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/abartrim/sobs/internal/features/defaultstore"
)

type Asset struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

// TokenClaims holds the claims encoded in a RUM client token, matching
// Python's _rum_client_token_encode / _rum_client_token_decode.
type TokenClaims struct {
	Issuer  string `json:"iss"`
	App     string `json:"app"`
	Origin  string `json:"origin"`
	IssuedAt int64 `json:"iat"`
	Expires int64  `json:"exp"`
	JTIID   string `json:"jti"`
}

type Service struct {
	assetDir string
}

func NewService() *Service {
	return NewFileService(defaultstore.NewDir("sobs-rum-assets-"))
}

func NewFileService(assetDir string) *Service {
	_ = os.MkdirAll(assetDir, 0o755)
	return &Service{assetDir: assetDir}
}

func (s *Service) CreateAsset(content string) (Asset, error) {
	return s.createFileBackedAsset(content)
}

func (s *Service) createFileBackedAsset(content string) (Asset, error) {
	if content == "" {
		return Asset{}, errors.New("content is required")
	}
	a := Asset{ID: newAssetID(), Content: content, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	body, err := json.Marshal(a)
	if err != nil {
		return Asset{}, err
	}
	if err := os.WriteFile(filepath.Join(s.assetDir, a.ID+".json"), body, 0o644); err != nil {
		return Asset{}, err
	}
	return a, nil
}

func (s *Service) GetAsset(id string) (Asset, bool) {
	return s.getFileBackedAsset(id)
}

func (s *Service) getFileBackedAsset(id string) (Asset, bool) {
	body, err := os.ReadFile(filepath.Join(s.assetDir, id+".json"))
	if err != nil {
		return Asset{}, false
	}
	var asset Asset
	if err := json.Unmarshal(body, &asset); err != nil {
		return Asset{}, false
	}
	return asset, true
}

// AuthMode returns SOBS_RUM_CLIENT_AUTH_MODE (default "none").
func (s *Service) AuthMode() string {
	return strings.ToLower(strings.TrimSpace(os.Getenv("SOBS_RUM_CLIENT_AUTH_MODE")))
}

// SigningKey returns SOBS_RUM_CLIENT_SIGNING_KEY.
func (s *Service) SigningKey() string {
	return os.Getenv("SOBS_RUM_CLIENT_SIGNING_KEY")
}

// TokenTTL returns SOBS_RUM_CLIENT_TOKEN_TTL_SEC (default 900).
func (s *Service) TokenTTL() int {
	v, err := strconv.Atoi(os.Getenv("SOBS_RUM_CLIENT_TOKEN_TTL_SEC"))
	if err != nil || v <= 0 {
		return 900
	}
	return v
}

// EncodeToken builds a signed token from claims, matching Python's
// _rum_client_token_encode: base64url(json(claims)) + "." + hmac_sha256_hex.
func (s *Service) EncodeToken(signingKey string, claims map[string]any) string {
	payload, _ := json.Marshal(claims)
	encoded := b64urlEncode(payload)
	sig := rumSign(signingKey, encoded)
	return encoded + "." + sig
}

// DecodeToken validates and decodes a signed token, matching Python's
// _rum_client_token_decode.
func (s *Service) DecodeToken(signingKey, token string) (map[string]any, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return nil, errors.New("invalid RUM client token format")
	}
	payloadB64, sig := parts[0], strings.ToLower(parts[1])
	expected := rumSign(signingKey, payloadB64)
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return nil, errors.New("invalid RUM client token signature")
	}
	raw, err := b64urlDecode(payloadB64)
	if err != nil {
		return nil, errors.New("invalid RUM client token payload")
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil, errors.New("invalid RUM client token payload")
	}
	return claims, nil
}

// NewClientToken issues a signed JWT-like token with the given claims,
// matching Python's issue_rum_client_token output.
// Returns the encoded token and expiry unix timestamp.
func (s *Service) NewClientToken(signingKey, origin, appName string, ttlSec int) (string, int64) {
	now := time.Now().Unix()
	exp := now + int64(ttlSec)
	claims := map[string]any{
		"iss":    "sobs-rum",
		"app":    appName,
		"origin": origin,
		"iat":    now,
		"exp":    exp,
		"jti":    newAssetID(),
	}
	return s.EncodeToken(signingKey, claims), exp
}

// rumSign computes HMAC-SHA256 hex of payload using signingKey,
// matching Python's _rum_client_sign.
func rumSign(signingKey, payload string) string {
	mac := hmac.New(sha256.New, []byte(signingKey))
	mac.Write([]byte(payload))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

func b64urlEncode(data []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}

func b64urlDecode(s string) ([]byte, error) {
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}

// newAssetID returns a random 32-char hex string used as an asset file ID.
func newAssetID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

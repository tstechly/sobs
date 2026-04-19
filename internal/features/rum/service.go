package rum

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/abartrim/sobs/internal/features/defaultstore"
)

type Asset struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

type Service struct {
	mu     sync.RWMutex
	assets map[string]Asset
	nextID int64
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
	if s.assetDir != "" {
		return s.createFileBackedAsset(content)
	}
	if content == "" {
		return Asset{}, errors.New("content is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := strconv.FormatInt(s.nextID, 10)
	s.nextID++
	a := Asset{ID: id, Content: content, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	s.assets[id] = a
	return a, nil
}

func (s *Service) createFileBackedAsset(content string) (Asset, error) {
	if content == "" {
		return Asset{}, errors.New("content is required")
	}
	a := Asset{ID: newClientToken(), Content: content, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
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
	if s.assetDir != "" {
		return s.getFileBackedAsset(id)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.assets[id]
	return a, ok
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

func (s *Service) NewClientToken() string {
	if s.assetDir != "" {
		return newClientToken()
	}
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func newClientToken() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

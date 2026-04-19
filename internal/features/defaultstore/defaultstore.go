package defaultstore

import (
	"os"
	"path/filepath"

	"github.com/abartrim/sobs/internal/extensionpoints"
	storepkg "github.com/abartrim/sobs/internal/store"
)

func NewFactory() extensionpoints.StoreFactory {
	tmpPath, err := os.MkdirTemp("", "sobs-feature-store-")
	if err != nil {
		return storepkg.NewChdbStoreFactory("")
	}
	return storepkg.NewChdbStoreFactory(tmpPath)
}

func NewDir(prefix string) string {
	tmpPath, err := os.MkdirTemp("", prefix)
	if err != nil {
		return filepath.Join(os.TempDir(), prefix)
	}
	return tmpPath
}
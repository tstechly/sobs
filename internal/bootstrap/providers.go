package bootstrap

import (
	"fmt"
	"os"
	"strings"

	"github.com/abartrim/sobs/internal/auth"
	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/store"
)

func BuildAuthProvider() (extensionpoints.AuthProvider, error) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SOBS_AUTH_PROVIDER"))) {
	case "", "static":
		return auth.NewStaticProvider(), nil
	default:
		return nil, fmt.Errorf("unknown auth provider")
	}
}

func BuildStoreFactory() (extensionpoints.StoreFactory, error) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SOBS_STORE_PROVIDER"))) {
	case "", "chdb":
		return store.NewChdbStoreFactoryFromEnv(), nil
	case "noop":
		return store.NewNoopStoreFactory(), nil
	default:
		return nil, fmt.Errorf("unknown store provider")
	}
}

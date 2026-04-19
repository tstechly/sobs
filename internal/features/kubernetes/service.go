package kubernetes

import (
	"context"
	"sync"

	"github.com/abartrim/sobs/internal/features/defaultstore"
	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/persist"
)

type Settings struct {
	Enabled         bool   `json:"enabled"`
	DefaultNamespace string `json:"default_namespace"`
}

type Service struct {
	mu       sync.RWMutex
	settings Settings
	storeFactory extensionpoints.StoreFactory
}

func NewService() *Service {
	return NewStoreService(defaultstore.NewFactory())
}

func NewStoreService(factory extensionpoints.StoreFactory) *Service {
	return &Service{storeFactory: factory}
}

func (s *Service) GetSettings() Settings {
	return Settings{
		Enabled:          kubernetesSettingBool(context.Background(), s.storeFactory, "kubernetes.enabled", true),
		DefaultNamespace: kubernetesSettingString(context.Background(), s.storeFactory, "kubernetes.default_namespace", "default"),
	}
}

func (s *Service) SaveSettings(enabled bool, namespace string) Settings {
	if namespace == "" {
		namespace = "default"
	}
	_ = persist.SetAppSetting(context.Background(), s.storeFactory, "kubernetes.enabled", kubernetesBoolString(enabled))
	_ = persist.SetAppSetting(context.Background(), s.storeFactory, "kubernetes.default_namespace", namespace)
	return Settings{Enabled: enabled, DefaultNamespace: namespace}
}

func (s *Service) Status() map[string]any {
	settings := s.GetSettings()
	if !settings.Enabled {
		return map[string]any{"ok": false, "error": "Kubernetes health view is disabled."}
	}
	return map[string]any{
		"ok": true,
		"summary": map[string]any{"pods_total": 0, "deployments_total": 0, "nodes_total": 0, "namespaces_total": 1},
		"pods": []map[string]any{},
		"deployments": []map[string]any{},
		"nodes": []map[string]any{},
		"namespaces": []map[string]any{{"name": settings.DefaultNamespace, "status": "Configured"}},
	}
}

func kubernetesBoolString(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func kubernetesSettingString(ctx context.Context, factory extensionpoints.StoreFactory, key, def string) string {
	value, ok, err := persist.GetAppSetting(ctx, factory, key)
	if err != nil || !ok || value == "" {
		return def
	}
	return value
}

func kubernetesSettingBool(ctx context.Context, factory extensionpoints.StoreFactory, key string, def bool) bool {
	value := kubernetesSettingString(ctx, factory, key, "")
	if value == "" {
		return def
	}
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

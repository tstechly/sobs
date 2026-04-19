package kubernetes

import (
	"context"
	"sync"

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
	return &Service{settings: Settings{Enabled: true, DefaultNamespace: "default"}}
}

func NewStoreService(factory extensionpoints.StoreFactory) *Service {
	return &Service{storeFactory: factory}
}

func (s *Service) GetSettings() Settings {
	if s.storeFactory != nil {
		return Settings{
			Enabled:          kubernetesSettingBool(context.Background(), s.storeFactory, "kubernetes.enabled", true),
			DefaultNamespace: kubernetesSettingString(context.Background(), s.storeFactory, "kubernetes.default_namespace", "default"),
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

func (s *Service) SaveSettings(enabled bool, namespace string) Settings {
	if s.storeFactory != nil {
		if namespace == "" {
			namespace = "default"
		}
		_ = persist.SetAppSetting(context.Background(), s.storeFactory, "kubernetes.enabled", kubernetesBoolString(enabled))
		_ = persist.SetAppSetting(context.Background(), s.storeFactory, "kubernetes.default_namespace", namespace)
		return Settings{Enabled: enabled, DefaultNamespace: namespace}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings.Enabled = enabled
	if namespace != "" {
		s.settings.DefaultNamespace = namespace
	}
	return s.settings
}

func (s *Service) Status() map[string]any {
	if s.storeFactory != nil {
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
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.settings.Enabled {
		return map[string]any{"ok": false, "error": "Kubernetes health view is disabled."}
	}
	return map[string]any{
		"ok": true,
		"summary": map[string]any{"pods_total": 5, "deployments_total": 3, "nodes_total": 2, "namespaces_total": 2},
		"pods": []map[string]any{{"name": "api-0", "namespace": s.settings.DefaultNamespace, "status": "Running"}},
		"deployments": []map[string]any{{"name": "api", "namespace": s.settings.DefaultNamespace, "ready": "1/1"}},
		"nodes": []map[string]any{{"name": "node-1", "status": "Ready"}},
		"namespaces": []map[string]any{{"name": s.settings.DefaultNamespace, "status": "Active"}},
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

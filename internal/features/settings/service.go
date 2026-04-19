package settings

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/persist"
)

type Service struct {
	mu         sync.RWMutex
	ai         map[string]string
	enrichment map[string]string
	storeFactory extensionpoints.StoreFactory
}

func NewService() *Service {
	return &Service{
		ai: map[string]string{},
		enrichment: map[string]string{
			"geo_enabled":                  "true",
			"cve_enabled":                  "true",
			"github_backfill_max_releases": "50",
		},
	}
}

func NewStoreService(factory extensionpoints.StoreFactory) *Service {
	return &Service{storeFactory: factory}
}

func (s *Service) SaveAI(values map[string]string) {
	if s.storeFactory != nil {
		_ = persist.SetAppSetting(context.Background(), s.storeFactory, "settings.ai", persist.JSONString(values))
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range values {
		s.ai[k] = strings.TrimSpace(v)
	}
}

func (s *Service) AI() map[string]string {
	if s.storeFactory != nil {
		if raw, ok, err := persist.GetAppSetting(context.Background(), s.storeFactory, "settings.ai"); err == nil && ok {
			return persist.ParseStringMap(raw)
		}
		return map[string]string{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.ai))
	for k, v := range s.ai {
		out[k] = v
	}
	return out
}

func (s *Service) SaveEnrichment(geoEnabled, cveEnabled bool, maxReleases int) {
	if maxReleases < 1 {
		maxReleases = 1
	}
	if maxReleases > 500 {
		maxReleases = 500
	}
	if s.storeFactory != nil {
		values := map[string]string{
			"geo_enabled":                  strconv.FormatBool(geoEnabled),
			"cve_enabled":                  strconv.FormatBool(cveEnabled),
			"github_backfill_max_releases": strconv.Itoa(maxReleases),
		}
		_ = persist.SetAppSetting(context.Background(), s.storeFactory, "settings.enrichment", persist.JSONString(values))
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if geoEnabled {
		s.enrichment["geo_enabled"] = "true"
	} else {
		s.enrichment["geo_enabled"] = "false"
	}
	if cveEnabled {
		s.enrichment["cve_enabled"] = "true"
	} else {
		s.enrichment["cve_enabled"] = "false"
	}
	s.enrichment["github_backfill_max_releases"] = strconv.Itoa(maxReleases)
}

func (s *Service) Enrichment() map[string]string {
	if s.storeFactory != nil {
		if raw, ok, err := persist.GetAppSetting(context.Background(), s.storeFactory, "settings.enrichment"); err == nil && ok {
			values := persist.ParseStringMap(raw)
			if len(values) > 0 {
				return values
			}
		}
		return map[string]string{
			"geo_enabled":                  "true",
			"cve_enabled":                  "true",
			"github_backfill_max_releases": "50",
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.enrichment))
	for k, v := range s.enrichment {
		out[k] = v
	}
	return out
}

func SortedActions(actions map[string]bool) []string {
	out := make([]string, 0, len(actions))
	for k, enabled := range actions {
		if enabled {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	if len(out) == 0 {
		return []string{"analyze"}
	}
	return out
}

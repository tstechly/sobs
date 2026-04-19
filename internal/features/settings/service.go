package settings

import (
	"context"
	"sort"
	"strconv"
	"sync"

	"github.com/abartrim/sobs/internal/features/defaultstore"
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
	return NewStoreService(defaultstore.NewFactory())
}

func NewStoreService(factory extensionpoints.StoreFactory) *Service {
	return &Service{storeFactory: factory}
}

func (s *Service) SaveAI(values map[string]string) {
	_ = persist.SetAppSetting(context.Background(), s.storeFactory, "settings.ai", persist.JSONString(values))
}

func (s *Service) AI() map[string]string {
	if raw, ok, err := persist.GetAppSetting(context.Background(), s.storeFactory, "settings.ai"); err == nil && ok {
		return persist.ParseStringMap(raw)
	}
	return map[string]string{}
}

func (s *Service) SaveEnrichment(geoEnabled, cveEnabled bool, maxReleases int) {
	if maxReleases < 1 {
		maxReleases = 1
	}
	if maxReleases > 500 {
		maxReleases = 500
	}
	values := map[string]string{
		"geo_enabled":                  strconv.FormatBool(geoEnabled),
		"cve_enabled":                  strconv.FormatBool(cveEnabled),
		"github_backfill_max_releases": strconv.Itoa(maxReleases),
	}
	_ = persist.SetAppSetting(context.Background(), s.storeFactory, "settings.enrichment", persist.JSONString(values))
}

func (s *Service) Enrichment() map[string]string {
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

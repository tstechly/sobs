package web

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/abartrim/sobs/internal/config"
	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/agents"
	"github.com/abartrim/sobs/internal/features/ai"
	"github.com/abartrim/sobs/internal/features/apps"
	"github.com/abartrim/sobs/internal/features/dashboards"
	"github.com/abartrim/sobs/internal/features/datamanagement"
	"github.com/abartrim/sobs/internal/features/enrichment"
	"github.com/abartrim/sobs/internal/features/kubernetes"
	"github.com/abartrim/sobs/internal/features/masking"
	"github.com/abartrim/sobs/internal/features/mcp"
	"github.com/abartrim/sobs/internal/features/metrics"
	"github.com/abartrim/sobs/internal/features/notifications"
	"github.com/abartrim/sobs/internal/features/onboarding"
	"github.com/abartrim/sobs/internal/features/repositories"
	"github.com/abartrim/sobs/internal/features/reports"
	"github.com/abartrim/sobs/internal/features/rum"
	"github.com/abartrim/sobs/internal/features/settings"
	"github.com/abartrim/sobs/internal/features/tags"
	"github.com/abartrim/sobs/internal/features/workitems"
	"github.com/abartrim/sobs/internal/ingest/otlpreceiver"
	storepkg "github.com/abartrim/sobs/internal/store"
	"github.com/abartrim/sobs/internal/templates"
	"github.com/flosch/pongo2/v6"
)

type Server struct {
	cfg                 config.Config
	authProvider        extensionpoints.AuthProvider
	storeFactory        extensionpoints.StoreFactory
	renderer            *templates.Renderer
	renderErr           error
	otlpHTTP            *otlpreceiver.HTTPServer
	appService          *apps.Service
	workItemService     *workitems.Service
	aiService           *ai.Service
	agentService        *agents.Service
	rumService          *rum.Service
	enrichmentService   *enrichment.Service
	kubernetesService   *kubernetes.Service
	dataManagementService *datamanagement.Service
	onboardingService   *onboarding.Service
	maskingService      *masking.Service
	mcpService          *mcp.Service
	metricsService      *metrics.Service
	tagService          *tags.Service
	notificationService *notifications.Service
	repositoryService   *repositories.Service
	settingsService     *settings.Service
	dashboardService    *dashboards.Service
	reportService       *reports.Service
}

func NewServer(cfg config.Config, authProvider extensionpoints.AuthProvider, storeFactory extensionpoints.StoreFactory) *Server {
	renderer, err := templates.NewRenderer(cfg.TemplateRoot)
	if _, ok := storeFactory.(*storepkg.NoopStoreFactory); ok {
		if tmpPath, tmpErr := os.MkdirTemp("", "sobs-chdb-"); tmpErr == nil {
			storeFactory = storepkg.NewChdbStoreFactory(tmpPath)
		} else {
			storeFactory = storepkg.NewChdbStoreFactory("")
		}
	}
	appService := apps.NewStoreService(storeFactory)
	workItemService := workitems.NewStoreService(storeFactory)
	aiService := ai.NewStoreService(storeFactory)
	agentService := agents.NewStoreService(storeFactory)
	rumService := rum.NewFileService("data/rum_assets")
	enrichmentService := enrichment.NewStoreService(storeFactory)
	kubernetesService := kubernetes.NewStoreService(storeFactory)
	metricsService := metrics.NewStoreService(storeFactory)
	tagService := tags.NewStoreService(storeFactory)
	notificationService := notifications.NewStoreService(storeFactory)
	repositoryService := repositories.NewStoreService(storeFactory)
	settingsService := settings.NewStoreService(storeFactory)
	dashboardService := dashboards.NewStoreService(storeFactory)
	reportService := reports.NewStoreService(storeFactory)
	dataManagementService := datamanagement.NewStoreService(storeFactory)
	onboardingService := onboarding.NewStoreService(storeFactory)
	maskingService := masking.NewStoreService(storeFactory)
	mcpService := mcp.NewStoreService(storeFactory)
	otlpHTTP := otlpreceiver.NewHTTPServerWithPipeline(otlpreceiver.NewStorePipeline(storeFactory))
	return &Server{
		cfg:                 cfg,
		authProvider:        authProvider,
		storeFactory:        storeFactory,
		renderer:            renderer,
		renderErr:           err,
		otlpHTTP:            otlpHTTP,
		appService:          appService,
		workItemService:     workItemService,
		aiService:           aiService,
		agentService:        agentService,
		rumService:          rumService,
		enrichmentService:   enrichmentService,
		kubernetesService:   kubernetesService,
		dataManagementService: dataManagementService,
		onboardingService:   onboardingService,
		maskingService:      maskingService,
		mcpService:          mcpService,
		metricsService:      metricsService,
		tagService:          tagService,
		notificationService: notificationService,
		repositoryService:   repositoryService,
		settingsService:     settingsService,
		dashboardService:    dashboardService,
		reportService:       reportService,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	staticRoot := resolveAssetRoot("static", "bootstrap.min.css")
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticRoot))))
	mux.HandleFunc("/", s.root)
	mux.HandleFunc("/health", s.healthz)
	mux.HandleFunc("/health/db", s.readyz)
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/readyz", s.readyz)
	mux.HandleFunc("/go/smoke", s.goSmoke)
	mux.HandleFunc("/auth/session", s.session)
	mux.HandleFunc("/mcp/tools", s.mcpListTools)
	mux.HandleFunc("/mcp", s.mcpEndpoint)
	mux.HandleFunc("/v1/apps", s.v1Apps)
	mux.HandleFunc("/v1/apps/", s.v1AppsSubroutes)
	mux.HandleFunc("/v1/releases/", s.v1ReleasesSubroutes)
	mux.HandleFunc("/api/reports", s.apiReports)
	mux.HandleFunc("/api/reports/", s.apiReportsSubroutes)
	mux.HandleFunc("/api/reports/export", s.apiReportsExport)
	mux.HandleFunc("/api/reports/import", s.apiReportsImport)
	mux.HandleFunc("/api/logs/list", s.apiLogsList)
	mux.HandleFunc("/api/logs/options", s.apiLogsOptions)
	mux.HandleFunc("/api/errors/list", s.apiErrorsList)
	mux.HandleFunc("/api/errors/options", s.apiErrorsOptions)
	mux.HandleFunc("/api/traces/list", s.apiTracesList)
	mux.HandleFunc("/api/traces/options", s.apiTracesOptions)
	mux.HandleFunc("/api/metrics/options", s.apiMetricsOptions)
	mux.HandleFunc("/api/dashboards/list", s.apiDashboardsList)
	mux.HandleFunc("/api/dashboards/query", s.apiDashboardsQuery)
	mux.HandleFunc("/api/dashboards/spec/templates", s.apiDashboardsSpecTemplates)
	mux.HandleFunc("/api/dashboards/spec/options", s.apiDashboardsSpecOptions)
	mux.HandleFunc("/api/dashboards/spec/compile", s.apiDashboardsSpecCompile)
	mux.HandleFunc("/api/dashboards/spec/dry-run", s.apiDashboardsSpecDryRun)
	mux.HandleFunc("/api/dashboards/spec/validate", s.apiDashboardsSpecValidate)
	mux.HandleFunc("/api/dashboards/spec/render", s.apiDashboardsSpecRender)
	mux.HandleFunc("/api/dashboards/render", s.apiDashboardsRender)
	mux.HandleFunc("/api/dashboards/spec/ai-build", s.apiDashboardsSpecAIBuild)
	mux.HandleFunc("/api/dashboards/", s.apiDashboardsChartSubroutes)
	mux.HandleFunc("/dashboards", s.dashboardsRoot)
	mux.HandleFunc("/dashboards/new", s.dashboardsNew)
	mux.HandleFunc("/dashboards/", s.dashboardsSubroutes)
	mux.HandleFunc("/api/query/add-to-dashboard", s.apiQueryAddToDashboard)
	mux.HandleFunc("/api/mcp/keys", s.apiMCPKeys)
	mux.HandleFunc("/api/mcp/keys/", s.apiMCPKeySubroutes)
	mux.HandleFunc("/api/mcp/enabled", s.apiMCPEnabled)
	mux.HandleFunc("/reports/", s.reportsPageDelete)
	mux.HandleFunc("/api/work-items", s.apiWorkItems)
	mux.HandleFunc("/api/ai/conversation", s.apiAIConversation)
	mux.HandleFunc("/api/ai/span-attributes", s.apiAISpanAttributes)
	mux.HandleFunc("/api/ai/export", s.apiAIExport)
	mux.HandleFunc("/api/ai/helper/capabilities", s.apiAIHelperCapabilities)
	mux.HandleFunc("/api/ai/helper/actions/manifest", s.apiAIHelperActionsManifest)
	mux.HandleFunc("/api/ai/helper/chats", s.apiAIHelperChats)
	mux.HandleFunc("/api/ai/helper/chats/", s.apiAIHelperChatByID)
	mux.HandleFunc("/api/ai/helper/feedback", s.apiAIHelperFeedback)
	mux.HandleFunc("/api/ai/helper", s.apiAIHelper)
	mux.HandleFunc("/api/ai/helper/actions/execute", s.apiAIHelperActionsExecute)
	mux.HandleFunc("/api/issues/raise", s.apiIssuesRaise)
	mux.HandleFunc("/api/agent/runs", s.apiAgentRuns)
	mux.HandleFunc("/api/agent/runs/", s.apiAgentRunsSubroutes)
	mux.HandleFunc("/api/query/ask", s.apiQueryAsk)
	mux.HandleFunc("/api/query/run", s.apiQueryRun)
	mux.HandleFunc("/api/query/refine-chart", s.apiQueryRefineChart)
	mux.HandleFunc("/api/query/schema", s.apiQuerySchema)
	mux.HandleFunc("/table-explorer", s.tableExplorerPage)
	mux.HandleFunc("/table-explorer/help", s.tableExplorerHelpPage)
	mux.HandleFunc("/api/table-explorer/tables", s.apiTableExplorerTables)
	mux.HandleFunc("/api/table-explorer/table/", s.apiTableExplorerTable)
	mux.HandleFunc("/api/chart-types", s.apiChartTypes)
	mux.HandleFunc("/api/web-traffic/geo", s.apiWebTrafficGeo)
	mux.HandleFunc("/api/web-traffic/browsers", s.apiWebTrafficBrowsers)
	mux.HandleFunc("/api/web-traffic/os", s.apiWebTrafficOS)
	mux.HandleFunc("/api/web-traffic/timezones", s.apiWebTrafficTimezones)
	mux.HandleFunc("/api/web-traffic/languages", s.apiWebTrafficLanguages)
	mux.HandleFunc("/api/web-traffic/devices", s.apiWebTrafficDevices)
	mux.HandleFunc("/api/enrichment/libraries", s.apiEnrichmentLibraries)
	mux.HandleFunc("/api/enrichment/github/repo-health", s.apiEnrichmentGitHubRepoHealth)
	mux.HandleFunc("/api/enrichment/cve/findings", s.apiEnrichmentCVEFindings)
	mux.HandleFunc("/api/enrichment/cve/findings/", s.apiEnrichmentCVEFindingsSubroutes)
	mux.HandleFunc("/api/enrichment/cve/scan", s.apiEnrichmentCVEScan)
	mux.HandleFunc("/enrichment/cve", s.enrichmentCVEPage)
	mux.HandleFunc("/settings/kubernetes", s.settingsKubernetes)
	mux.HandleFunc("/kubernetes", s.kubernetesPage)
	mux.HandleFunc("/api/kubernetes/status", s.apiKubernetesStatus)
	mux.HandleFunc("/settings/data-management", s.settingsDataManagement)
	mux.HandleFunc("/api/data-management/backup/list", s.apiDataManagementBackupList)
	mux.HandleFunc("/api/data-management/backup/run", s.apiDataManagementBackupRun)
	mux.HandleFunc("/api/data-management/restore", s.apiDataManagementRestore)
	mux.HandleFunc("/api/setup-wizard/steps", s.apiSetupWizardSteps)
	mux.HandleFunc("/api/onboarding/create-repo", s.apiOnboardingCreateRepo)
	mux.HandleFunc("/api/onboarding/import-repo", s.apiOnboardingImportRepo)
	mux.HandleFunc("/api/onboarding/list-repos", s.apiOnboardingListRepos)
	mux.HandleFunc("/api/onboarding/inspect-repo", s.apiOnboardingInspectRepo)
	mux.HandleFunc("/api/onboarding/create-issues", s.apiOnboardingCreateIssues)
	mux.HandleFunc("/metrics/rules", s.metricsRules)
	mux.HandleFunc("/metrics/rules/auto", s.metricsRulesAuto)
	mux.HandleFunc("/metrics/rules/dashboard/auto", s.metricsRulesDashboardAuto)
	mux.HandleFunc("/metrics/rules/", s.metricsRulesSubroutes)
	mux.HandleFunc("/metrics/anomaly", s.metricsAnomalyPage)
	mux.HandleFunc("/api/metrics/anomaly", s.apiMetricsAnomaly)
	mux.HandleFunc("/api/metrics/summary", s.apiMetricsSummary)
	mux.HandleFunc("/api/metrics/timeseries", s.apiMetricsTimeseries)
	mux.HandleFunc("/api/logs/field-hints", s.apiLogsFieldHints)
	mux.HandleFunc("/api/ai/field-hints", s.apiAIFieldHints)
	mux.HandleFunc("/api/logs/validate-filter", s.apiLogsValidateFilter)
	mux.HandleFunc("/api/ai/validate-filter", s.apiAIValidateFilter)
	mux.HandleFunc("/api/logs/validate-regex", s.apiLogsValidateRegex)
	mux.HandleFunc("/api/errors/validate-regex", s.apiErrorsValidateRegex)
	mux.HandleFunc("/api/traces/validate-regex", s.apiTracesValidateRegex)
	mux.HandleFunc("/api/metrics/validate-regex", s.apiMetricsValidateRegex)
	mux.HandleFunc("/api/rum/validate-regex", s.apiRUMValidateRegex)
	mux.HandleFunc("/v1/rum/assets", s.v1RUMAssets)
	mux.HandleFunc("/v1/rum/assets/", s.v1RUMAssetByID)
	mux.HandleFunc("/v1/rum/client-token", s.v1RUMClientToken)
	mux.HandleFunc("/metrics", s.metricsPage)
	mux.HandleFunc("/errors/", s.errorsResolve)
	mux.HandleFunc("/api/traces/span/", s.apiTraceSpan)
	mux.HandleFunc("/api/notifications/check", s.notificationsCheck)
	mux.HandleFunc("/api/notifications/vapid-public-key", s.apiNotificationsVapidPublicKey)
	mux.HandleFunc("/api/notifications/subscribe", s.apiNotificationsSubscribe)
	mux.HandleFunc("/api/notifications/vapid-keygen", s.apiNotificationsVAPIDKeygen)
	mux.HandleFunc("/api/notifications/vapid-keys", s.apiNotificationsVAPIDKeysDelete)
	mux.HandleFunc("/api/notifications/rules", s.apiNotificationsRules)
	mux.HandleFunc("/api/notifications/subscriptions", s.apiNotificationsSubscriptions)
	mux.HandleFunc("/api/notifications/channels/", s.apiNotificationsChannelSubroutes)
	mux.HandleFunc("/api/notifications/rules/auto-generate", s.apiNotificationsRulesAutoGenerate)
	mux.HandleFunc("/settings/repositories", s.settingsRepositories)
	mux.HandleFunc("/settings/repositories/github-token/validate", s.settingsRepositoriesValidateToken)
	mux.HandleFunc("/settings/repositories/", s.settingsRepositoriesSubroutes)
	mux.HandleFunc("/settings/ai", s.settingsAI)
	mux.HandleFunc("/settings/enrichment", s.settingsEnrichment)
	mux.HandleFunc("/settings/agents", s.settingsAgents)
	mux.HandleFunc("/settings/agents/", s.settingsAgentsSubroutes)
	mux.HandleFunc("/settings/masking", s.settingsMaskingPage)
	mux.HandleFunc("/settings/masking/keys", s.settingsMaskingKeysCreate)
	mux.HandleFunc("/settings/masking/keys/delete", s.settingsMaskingKeysDelete)
	mux.HandleFunc("/settings/masking/patterns", s.settingsMaskingPatternsCreate)
	mux.HandleFunc("/settings/masking/patterns/delete", s.settingsMaskingPatternsDelete)
	mux.HandleFunc("/settings/masking/output", s.settingsMaskingOutput)
	mux.HandleFunc("/settings/masking/sql-output", s.settingsMaskingSQLOutput)
	mux.HandleFunc("/api/settings/masking/preview", s.apiSettingsMaskingPreview)
	mux.HandleFunc("/api/settings/masking/rules", s.apiSettingsMaskingRules)
	mux.HandleFunc("/settings/tags", s.settingsTags)
	mux.HandleFunc("/settings/tags/auto", s.settingsTagsAuto)
	mux.HandleFunc("/settings/tags/", s.settingsTagsSubroutes)
	mux.HandleFunc("/settings/mcp", s.settingsMCPPage)
	mux.HandleFunc("/api/settings/tags/condition-suggestions", s.apiSettingsTagsConditionSuggestions)
	mux.HandleFunc("/api/tags/", s.apiTagsRecord)
	mux.HandleFunc("/settings/notifications/channels", s.settingsNotificationsChannelsCreate)
	mux.HandleFunc("/settings/notifications/channels/", s.settingsNotificationsChannelActions)
	mux.HandleFunc("/settings/notifications/rules", s.settingsNotificationsRulesCreate)
	mux.HandleFunc("/settings/notifications/rules/", s.settingsNotificationsRulesActions)
	mux.HandleFunc("/tail", s.tail)
	mux.HandleFunc("/service-worker.js", s.serviceWorker)
	mux.HandleFunc("/static/rum.js", s.rumJS)
	mux.HandleFunc("/static/rum.js.map", s.rumJSMap)
	mux.HandleFunc("/static/rum.min.js", s.rumMinJS)
	mux.HandleFunc("/static/rum.min.js.map", s.rumMinJSMap)
	mux.HandleFunc("/static/rum.d.ts", s.rumDTS)
	s.otlpHTTP.Register(mux)
	s.registerPageRoutes(mux)
	return s.wrapSecurity(mux)
}

func resolveAssetRoot(root string, requiredFile string) string {
	resolved := root
	if filepath.IsAbs(root) {
		return root
	}
	cwd, err := os.Getwd()
	if err != nil {
		return resolved
	}
	probe := cwd
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(probe, root)
		if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
			if requiredFile == "" {
				return candidate
			}
			if fInfo, fErr := os.Stat(filepath.Join(candidate, requiredFile)); fErr == nil && !fInfo.IsDir() {
				return candidate
			}
		}
		next := filepath.Dir(probe)
		if next == probe {
			break
		}
		probe = next
	}
	return resolved
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) root(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if s.renderErr != nil || s.renderer == nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	body, err := s.renderer.Render("summary.html", pongo2.Context{
		"title":                 "Summary",
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": "summary"},
		"stats": map[string]any{
			"logs":     0,
			"errors":   0,
			"spans":    0,
			"rum":      0,
			"ai":       0,
			"services": []any{},
		},
		"signal_health": []any{},
		"recent_errors": []any{},
		"recent_logs":   []any{},
		"rum_summary":   []any{},
		"ai_summary":    []any{},
		"cve_overview": map[string]any{
			"enabled": false,
			"total":   0,
		},
	})
	if err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	if !sameOriginRequest(r, s.cfg.TrustedProxyMode) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	token := sessionTokenFromRequest(r, s.cfg.SessionCookieName)
	if token == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"session": token, "subject": ""})
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	store, err := s.storeFactory.Open(context.Background())
	if err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	defer func() {
		_ = store.Close()
	}()
	if err := store.Ping(r.Context()); err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}

func (s *Server) goSmoke(w http.ResponseWriter, r *http.Request) {
	if s.renderErr != nil || s.renderer == nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	body, err := s.renderer.Render("go_smoke.html", pongo2.Context{
		"title":   "SOBS Go Migration",
		"message": "Template rendering is active.",
	})
	if err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

func (s *Server) notificationsCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	channels := s.notificationService.ListSubscriptions()
	rules := s.notificationService.ListRules()
	activeRules := 0
	for _, item := range rules {
		if item.Enabled {
			activeRules++
		}
	}
	triggered := 0
	if len(channels) > 0 && activeRules > 0 {
		triggered = activeRules
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "channels": len(channels), "rules": len(rules), "triggered": triggered})
}

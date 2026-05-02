package setupwizard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	sobshttp "github.com/sobs/sobs-api/internal/http"
)

const version = "1"

var (
	wizardEnvs        = []string{"dev", "prod"}
	wizardLanguages   = []string{"python", "node", "go", "java", "dotnet", "ruby", "php"}
	wizardDeployments = []string{"docker", "kubernetes", "baremetal", "cloud"}
)

type Handler struct{}

type Response struct {
	OK         bool            `json:"ok"`
	Version    string          `json:"version,omitempty"`
	Env        string          `json:"env,omitempty"`
	Language   string          `json:"language,omitempty"`
	Deployment string          `json:"deployment,omitempty"`
	Steps      []Step          `json:"steps,omitempty"`
	Checklist  []ChecklistItem `json:"checklist,omitempty"`
	Error      string          `json:"error,omitempty"`
}

type Step struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Commands    []string `json:"commands"`
	Language    string   `json:"language"`
}

type ChecklistItem struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) Steps(w http.ResponseWriter, r *http.Request) {
	env := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("env")))
	if env == "" {
		env = "dev"
	}
	language := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("language")))
	if language == "" {
		language = "python"
	}
	deployment := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("deployment")))
	if deployment == "" {
		deployment = "docker"
	}

	if !contains(wizardEnvs, env) {
		sobshttp.JSON(w, http.StatusBadRequest, Response{OK: false, Error: fmt.Sprintf("Invalid env '%s'. Must be one of: %s", env, formatChoices(wizardEnvs))})
		return
	}
	if !contains(wizardLanguages, language) {
		sobshttp.JSON(w, http.StatusBadRequest, Response{OK: false, Error: fmt.Sprintf("Invalid language '%s'. Must be one of: %s", language, formatChoices(wizardLanguages))})
		return
	}
	if !contains(wizardDeployments, deployment) {
		sobshttp.JSON(w, http.StatusBadRequest, Response{OK: false, Error: fmt.Sprintf("Invalid deployment '%s'. Must be one of: %s", deployment, formatChoices(wizardDeployments))})
		return
	}

	result := buildSetupWizardSteps(env, language, deployment)
	sobshttp.JSON(w, http.StatusOK, Response{
		OK:         true,
		Version:    result.Version,
		Env:        result.Env,
		Language:   result.Language,
		Deployment: result.Deployment,
		Steps:      result.Steps,
		Checklist:  result.Checklist,
	})
}

type wizardResult struct {
	Version    string
	Env        string
	Language   string
	Deployment string
	Steps      []Step
	Checklist  []ChecklistItem
}

func buildSetupWizardSteps(env, language, deployment string) wizardResult {
	prod := env == "prod"
	var sdkSteps []Step

	switch language {
	case "python":
		pkgs := "opentelemetry-sdk opentelemetry-exporter-otlp opentelemetry-instrumentation"
		sdkSteps = append(sdkSteps,
			Step{
				ID:          "sdk_install",
				Title:       "Install OpenTelemetry Python SDK",
				Description: "Add the core SDK and OTLP exporter to your project.",
				Commands:    []string{fmt.Sprintf("pip install %s", pkgs)},
				Language:    "bash",
			},
			Step{
				ID:          "sdk_init",
				Title:       "Initialise SDK in your application",
				Description: "Bootstrap tracing and metrics at startup.",
				Commands: []string{
					"from opentelemetry import trace",
					"from opentelemetry.sdk.trace import TracerProvider",
					"from opentelemetry.sdk.trace.export import BatchSpanProcessor",
					"from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter",
					"",
					"provider = TracerProvider()",
					"provider.add_span_processor(",
					"    BatchSpanProcessor(OTLPSpanExporter(endpoint=\"http://localhost:4317\", insecure=True))",
					")",
					"trace.set_tracer_provider(provider)",
				},
				Language: "python",
			},
		)
	case "node":
		sdkSteps = append(sdkSteps,
			Step{
				ID:          "sdk_install",
				Title:       "Install OpenTelemetry Node.js SDK",
				Description: "Add the SDK and OTLP exporter packages.",
				Commands: []string{
					"npm install @opentelemetry/sdk-node @opentelemetry/auto-instrumentations-node @opentelemetry/exporter-trace-otlp-grpc",
				},
				Language: "bash",
			},
			Step{
				ID:          "sdk_init",
				Title:       "Initialise SDK (tracing.js)",
				Description: "Create tracing.js and require it before your app entry.",
				Commands: []string{
					"// tracing.js",
					"const { NodeSDK } = require('@opentelemetry/sdk-node');",
					"const { getNodeAutoInstrumentations } = require('@opentelemetry/auto-instrumentations-node');",
					"const { OTLPTraceExporter } = require('@opentelemetry/exporter-trace-otlp-grpc');",
					"",
					"const sdk = new NodeSDK({",
					"  traceExporter: new OTLPTraceExporter({ url: 'http://localhost:4317' }),",
					"  instrumentations: [getNodeAutoInstrumentations()],",
					"});",
					"sdk.start();",
				},
				Language: "javascript",
			},
		)
	case "go":
		sdkSteps = append(sdkSteps,
			Step{
				ID:          "sdk_install",
				Title:       "Add OpenTelemetry Go dependencies",
				Description: "Fetch the SDK and OTLP gRPC exporter modules.",
				Commands: []string{
					"go get go.opentelemetry.io/otel",
					"go get go.opentelemetry.io/otel/sdk/trace",
					"go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc",
				},
				Language: "bash",
			},
			Step{
				ID:          "sdk_init",
				Title:       "Initialise tracer provider",
				Description: "Wire the OTLP exporter into your main function.",
				Commands: []string{
					"exp, _ := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure(), otlptracegrpc.WithEndpoint(\"localhost:4317\"))",
					"tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp))",
					"otel.SetTracerProvider(tp)",
					"defer tp.Shutdown(ctx)",
				},
				Language: "go",
			},
		)
	case "java":
		sdkSteps = append(sdkSteps,
			Step{
				ID:          "sdk_install",
				Title:       "Add OpenTelemetry Java dependencies (Maven)",
				Description: "Add the OTLP exporter and SDK to your pom.xml.",
				Commands: []string{
					"<dependency>",
					"  <groupId>io.opentelemetry</groupId>",
					"  <artifactId>opentelemetry-sdk</artifactId>",
					"  <version>1.36.0</version>",
					"</dependency>",
					"<dependency>",
					"  <groupId>io.opentelemetry</groupId>",
					"  <artifactId>opentelemetry-exporter-otlp</artifactId>",
					"  <version>1.36.0</version>",
					"</dependency>",
				},
				Language: "xml",
			},
			Step{
				ID:          "sdk_init",
				Title:       "Alternatively use the Java agent (zero-code)",
				Description: "Attach the agent JAR to your JVM startup for automatic instrumentation.",
				Commands: []string{
					"# Download the agent",
					"curl -Lo opentelemetry-javaagent.jar https://github.com/open-telemetry/opentelemetry-java-instrumentation/releases/latest/download/opentelemetry-javaagent.jar",
					"",
					"# Run your app with the agent",
					"java -javaagent:opentelemetry-javaagent.jar -Dotel.exporter.otlp.endpoint=http://localhost:4317 -jar your-app.jar",
				},
				Language: "bash",
			},
		)
	case "dotnet":
		sdkSteps = append(sdkSteps,
			Step{
				ID:          "sdk_install",
				Title:       "Add OpenTelemetry .NET packages",
				Description: "Install the SDK and OTLP exporter via NuGet.",
				Commands: []string{
					"dotnet add package OpenTelemetry",
					"dotnet add package OpenTelemetry.Exporter.OpenTelemetryProtocol",
					"dotnet add package OpenTelemetry.Extensions.Hosting",
					"dotnet add package OpenTelemetry.Instrumentation.AspNetCore",
				},
				Language: "bash",
			},
			Step{
				ID:          "sdk_init",
				Title:       "Register OpenTelemetry in Program.cs",
				Description: "Configure tracing with OTLP export in your startup code.",
				Commands: []string{
					"builder.Services.AddOpenTelemetry()",
					"  .WithTracing(b => b",
					"    .AddAspNetCoreInstrumentation()",
					"    .AddOtlpExporter(o => o.Endpoint = new Uri(\"http://localhost:4317\")));",
				},
				Language: "csharp",
			},
		)
	case "ruby":
		sdkSteps = append(sdkSteps,
			Step{
				ID:          "sdk_install",
				Title:       "Add OpenTelemetry Ruby gems",
				Description: "Add the SDK and OTLP exporter to your Gemfile.",
				Commands: []string{
					"gem 'opentelemetry-sdk'",
					"gem 'opentelemetry-exporter-otlp'",
					"gem 'opentelemetry-instrumentation-all'",
					"",
					"# then run:",
					"bundle install",
				},
				Language: "ruby",
			},
			Step{
				ID:          "sdk_init",
				Title:       "Configure the SDK",
				Description: "Initialise OTEL before your app boots.",
				Commands: []string{
					"require 'opentelemetry/sdk'",
					"require 'opentelemetry/exporter/otlp'",
					"require 'opentelemetry/instrumentation/all'",
					"",
					"OpenTelemetry::SDK.configure do |c|",
					"  c.service_name = 'my-service'",
					"  c.use_all",
					"end",
				},
				Language: "ruby",
			},
		)
	case "php":
		sdkSteps = append(sdkSteps,
			Step{
				ID:          "sdk_install",
				Title:       "Install OpenTelemetry PHP SDK",
				Description: "Add the SDK and OTLP exporter via Composer.",
				Commands: []string{
					"composer require open-telemetry/sdk open-telemetry/exporter-otlp",
				},
				Language: "bash",
			},
			Step{
				ID:          "sdk_init",
				Title:       "Bootstrap the SDK",
				Description: "Configure a tracer provider before handling requests.",
				Commands: []string{
					"use OpenTelemetry\\SDK\\Trace\\TracerProviderFactory;",
					"use OpenTelemetry\\Contrib\\Otlp\\OtlpHttpTransportFactory;",
					"",
					"$tracerProvider = (new TracerProviderFactory())->create();",
					"\\OpenTelemetry\\API\\Globals::registerInitializer(fn() => $tracerProvider);",
				},
				Language: "php",
			},
		)
	}

	sobsOTLPEndpoint := "http://sobs:44317"
	if !prod {
		sobsOTLPEndpoint = "http://localhost:44317"
	}

	var collectorSteps []Step
	switch deployment {
	case "docker":
		collectorSteps = []Step{
			{
				ID:          "collector_run",
				Title:       "Run the OpenTelemetry Collector (Docker)",
				Description: "Start the contrib collector with a minimal config wired to SOBS.",
				Commands: []string{
					"# otel-collector-config.yaml",
					"receivers:",
					"  otlp:",
					"    protocols:",
					"      grpc:",
					"        endpoint: 0.0.0.0:4317",
					"      http:",
					"        endpoint: 0.0.0.0:4318",
					"exporters:",
					"  otlphttp:",
					fmt.Sprintf("    endpoint: %s", sobsOTLPEndpoint),
					"service:",
					"  pipelines:",
					"    traces:",
					"      receivers: [otlp]",
					"      exporters: [otlphttp]",
					"    metrics:",
					"      receivers: [otlp]",
					"      exporters: [otlphttp]",
					"    logs:",
					"      receivers: [otlp]",
					"      exporters: [otlphttp]",
				},
				Language: "yaml",
			},
			{
				ID:          "collector_docker_run",
				Title:       "Start the collector container",
				Description: "Mount the config and expose OTLP ports.",
				Commands: []string{
					"docker run -d --name otel-collector \\",
					"  -p 4317:4317 -p 4318:4318 \\",
					"  -v $(pwd)/otel-collector-config.yaml:/etc/otelcol-contrib/config.yaml \\",
					"  otel/opentelemetry-collector-contrib:latest",
				},
				Language: "bash",
			},
		}
	case "kubernetes":
		collectorSteps = []Step{
			{
				ID:          "collector_k8s",
				Title:       "Deploy the OpenTelemetry Collector on Kubernetes",
				Description: "Apply a ConfigMap and Deployment that routes to SOBS.",
				Commands: []string{
					"# otel-collector-k8s.yaml",
					"apiVersion: v1",
					"kind: ConfigMap",
					"metadata:",
					"  name: otel-collector-config",
					"data:",
					"  config.yaml: |",
					"    receivers:",
					"      otlp:",
					"        protocols:",
					"          grpc:",
					"            endpoint: 0.0.0.0:4317",
					"    exporters:",
					"      otlphttp:",
					fmt.Sprintf("        endpoint: %s", sobsOTLPEndpoint),
					"    service:",
					"      pipelines:",
					"        traces:",
					"          receivers: [otlp]",
					"          exporters: [otlphttp]",
					"        metrics:",
					"          receivers: [otlp]",
					"          exporters: [otlphttp]",
					"        logs:",
					"          receivers: [otlp]",
					"          exporters: [otlphttp]",
					"---",
					"apiVersion: apps/v1",
					"kind: Deployment",
					"metadata:",
					"  name: otel-collector",
					"spec:",
					"  replicas: 1",
					"  selector:",
					"    matchLabels:",
					"      app: otel-collector",
					"  template:",
					"    metadata:",
					"      labels:",
					"        app: otel-collector",
					"    spec:",
					"      containers:",
					"      - name: otel-collector",
					"        image: otel/opentelemetry-collector-contrib:latest",
					"        args: ['--config=/etc/otelcol-contrib/config.yaml']",
					"        volumeMounts:",
					"        - name: config",
					"          mountPath: /etc/otelcol-contrib",
					"      volumes:",
					"      - name: config",
					"        configMap:",
					"          name: otel-collector-config",
				},
				Language: "yaml",
			},
			{
				ID:          "collector_k8s_apply",
				Title:       "Apply the manifest",
				Description: "Deploy the collector to your cluster.",
				Commands:    []string{"kubectl apply -f otel-collector-k8s.yaml"},
				Language:    "bash",
			},
		}
	case "cloud":
		collectorSteps = []Step{
			{
				ID:          "collector_cloud",
				Title:       "Configure a managed OTLP pipeline",
				Description: "Point your cloud provider's OTLP endpoint to forward to SOBS.",
				Commands: []string{
					"# For AWS Distro for OpenTelemetry (ADOT):",
					"# Set the exporter endpoint in your ADOT config to:",
					fmt.Sprintf("#   endpoint: %s", sobsOTLPEndpoint),
					"",
					"# For GCP OpenTelemetry Collector:",
					"# Override the exporter.endpoint in your otel-config.yaml to:",
					fmt.Sprintf("#   endpoint: %s", sobsOTLPEndpoint),
				},
				Language: "yaml",
			},
		}
	default:
		collectorSteps = []Step{
			{
				ID:          "collector_binary",
				Title:       "Run the OpenTelemetry Collector (binary)",
				Description: "Download and run the contrib collector directly.",
				Commands: []string{
					"# Download (Linux amd64):",
					"curl -LO https://github.com/open-telemetry/opentelemetry-collector-releases/releases/latest/download/otelcol-contrib_linux_amd64.tar.gz",
					"tar xzf otelcol-contrib_linux_amd64.tar.gz",
					"",
					"# Write config.yaml (same format as Docker example above)",
					"",
					"# Start:",
					"./otelcol-contrib --config=config.yaml",
				},
				Language: "bash",
			},
		}
	}

	sobsSteps := []Step{
		{
			ID:          "sobs_verify",
			Title:       "Verify data arrives in SOBS",
			Description: "Check the Summary page for incoming telemetry.",
			Commands: []string{
				fmt.Sprintf("# Open your browser and navigate to %s/", sobsOTLPEndpoint),
				"# The Summary card should show span, log, and metric counts within ~30 s.",
			},
			Language: "bash",
		},
	}
	if prod {
		sobsSteps = append(sobsSteps, Step{
			ID:          "sobs_anomaly",
			Title:       "Enable anomaly detection rules",
			Description: "Head to Settings → Anomaly Rules and add your first threshold rule.",
			Commands: []string{
				fmt.Sprintf("# Navigate to: %s/settings/anomaly-rules", sobsOTLPEndpoint),
				"# Click 'Add Rule' and choose a metric from your stack.",
			},
			Language: "bash",
		})
	}

	checklist := []ChecklistItem{
		{ID: "sdk", Label: "Install & initialise the SDK"},
		{ID: "collector", Label: "Run the OpenTelemetry Collector"},
		{ID: "verify", Label: "Verify data in SOBS"},
	}
	if prod {
		checklist = append(checklist, ChecklistItem{ID: "anomaly", Label: "Configure anomaly detection"})
	}

	steps := make([]Step, 0, len(sdkSteps)+len(collectorSteps)+len(sobsSteps))
	steps = append(steps, sdkSteps...)
	steps = append(steps, collectorSteps...)
	steps = append(steps, sobsSteps...)

	return wizardResult{
		Version:    version,
		Env:        env,
		Language:   language,
		Deployment: deployment,
		Steps:      steps,
		Checklist:  checklist,
	}
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func formatChoices(items []string) string {
	choices := append([]string(nil), items...)
	sort.Strings(choices)
	parts := make([]string, 0, len(choices))
	for _, choice := range choices {
		parts = append(parts, fmt.Sprintf("'%s'", choice))
	}
	return fmt.Sprintf("[%s]", strings.Join(parts, ", "))
}

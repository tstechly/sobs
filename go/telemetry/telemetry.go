// Package telemetry provides OpenTelemetry instrumentation for the SOBS integration tests.
package telemetry

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	// DefaultOTLPEndpoint is the default OTLP endpoint for SOBS
	DefaultOTLPEndpoint = "localhost:44317"
)

var (
	// Global tracer provider and meter provider
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider

	// Global tracer and meter
	tracer trace.Tracer
	meter  metric.Meter

	// Metrics instruments
	httpRequestCounter     metric.Int64Counter
	httpRequestDuration    metric.Float64Histogram
	testDuration           metric.Float64Histogram
	testPassCounter        metric.Int64Counter
	testFailCounter        metric.Int64Counter
	testSkipCounter        metric.Int64Counter
	dbQueryCounter         metric.Int64Counter
	dbQueryDuration        metric.Float64Histogram
	errorCounter           metric.Int64Counter
	requestBodySizeBytes   metric.Int64Histogram
	responseBodySizeBytes  metric.Int64Histogram
)

var (
	// Flag to track if telemetry is enabled
	telemetryEnabled = false
)

// Init initializes OpenTelemetry instrumentation with OTLP exporters.
func Init(serviceName, serviceVersion string, opts ...Option) error {
	cfg := &config{
		oTLPEndpoint: getEnvOrDefault("SOBS_OTEL_ENDPOINT", DefaultOTLPEndpoint),
		enabled:      true,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	if !cfg.enabled {
		log.Println("[telemetry] OpenTelemetry disabled - skipping initialization")
		// Initialize no-op instruments
		initNoOpInstruments()
		return nil
	}

	// Mark telemetry as enabled
	telemetryEnabled = true

	log.Printf("[telemetry] Initializing OpenTelemetry for service=%s version=%s endpoint=%s",
		serviceName, serviceVersion, cfg.oTLPEndpoint)

	// Create resource with service metadata
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
			semconv.TelemetrySDKLanguageGo,
			semconv.TelemetrySDKName("opentelemetry"),
			semconv.TelemetrySDKVersion("1.31.0"),
			attribute.String("deployment.environment", getEnvOrDefault("DEPLOYMENT_ENVIRONMENT", "development")),
			attribute.String("app.name", "sobs-integration-tests"),
			attribute.String("app.component", "integration-test-suite"),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create resource: %w", err)
	}

	// Initialize TracerProvider with OTLP Trace Exporter
	if err := initTracing(res, cfg); err != nil {
		return fmt.Errorf("failed to initialize tracing: %w", err)
	}

	// Initialize MeterProvider with OTLP Metric Exporter
	if err := initMetrics(res, cfg); err != nil {
		return fmt.Errorf("failed to initialize metrics: %w", err)
	}

	// Set global propagator for context propagation
	otel.SetTextMapPropagator(propagation.TraceContext{})

	log.Println("[telemetry] OpenTelemetry initialized successfully")

	return nil
}

// initNoOpInstruments initializes no-op instruments when telemetry is disabled.
func initNoOpInstruments() {
	// Set flag to indicate telemetry is not enabled
	telemetryEnabled = false
	// Instruments will simply be nil and all recording calls will be no-ops
}

// initTracing initializes tracing with OTLP exporter
func initTracing(res *resource.Resource, cfg *config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create OTLP trace exporter
	traceExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.oTLPEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}

	// Create tracer provider
	tracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	// Set as global tracer provider
	otel.SetTracerProvider(tracerProvider)

	// Get tracer for this package
	tracer = tracerProvider.Tracer("github.com/abartrim/sobs/go/telemetry",
		trace.WithInstrumentationVersion("1.0.0"),
	)

	return nil
}

// initMetrics initializes metrics with OTLP exporter
func initMetrics(res *resource.Resource, cfg *config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create OTLP metric exporter
	metricExporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(cfg.oTLPEndpoint),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return fmt.Errorf("failed to create OTLP metric exporter: %w", err)
	}

	// Create meter provider
	meterProvider = sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter,
			sdkmetric.WithInterval(10*time.Second),
		)),
		sdkmetric.WithResource(res),
	)

	// Set as global meter provider
	otel.SetMeterProvider(meterProvider)

	// Get meter for this package
	meter = meterProvider.Meter("github.com/abartrim/sobs/go/telemetry",
		metric.WithInstrumentationVersion("1.0.0"),
	)

	// Initialize metric instruments
	initInstruments(ctx)

	return nil
}

// initInstruments initializes all metric instruments
func initInstruments(ctx context.Context) {
	var err error

	// HTTP Request Counter
	httpRequestCounter, err = meter.Int64Counter("http.client.requests",
		metric.WithDescription("Number of HTTP requests made by integration tests"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		log.Printf("[telemetry] Warning: failed to create http.client.requests counter: %v", err)
	}

	// HTTP Request Duration
	httpRequestDuration, err = meter.Float64Histogram("http.client.request.duration",
		metric.WithDescription("Duration of HTTP requests made by integration tests"),
		metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000),
	)
	if err != nil {
		log.Printf("[telemetry] Warning: failed to create http.client.request.duration histogram: %v", err)
	}

	// Test Duration
	testDuration, err = meter.Float64Histogram("test.execution.duration",
		metric.WithDescription("Duration of test execution"),
		metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000),
	)
	if err != nil {
		log.Printf("[telemetry] Warning: failed to create test.execution.duration histogram: %v", err)
	}

	// Test Pass Counter
	testPassCounter, err = meter.Int64Counter("test.results.pass",
		metric.WithDescription("Number of passed tests"),
		metric.WithUnit("{test}"),
	)
	if err != nil {
		log.Printf("[telemetry] Warning: failed to create test.results.pass counter: %v", err)
	}

	// Test Fail Counter
	testFailCounter, err = meter.Int64Counter("test.results.fail",
		metric.WithDescription("Number of failed tests"),
		metric.WithUnit("{test}"),
	)
	if err != nil {
		log.Printf("[telemetry] Warning: failed to create test.results.fail counter: %v", err)
	}

	// Test Skip Counter
	testSkipCounter, err = meter.Int64Counter("test.results.skip",
		metric.WithDescription("Number of skipped tests"),
		metric.WithUnit("{test}"),
	)
	if err != nil {
		log.Printf("[telemetry] Warning: failed to create test.results.skip counter: %v", err)
	}

	// DB Query Counter
	dbQueryCounter, err = meter.Int64Counter("db.queries",
		metric.WithDescription("Number of database queries"),
		metric.WithUnit("{query}"),
	)
	if err != nil {
		log.Printf("[telemetry] Warning: failed to create db.queries counter: %v", err)
	}

	// DB Query Duration
	dbQueryDuration, err = meter.Float64Histogram("db.queries.duration",
		metric.WithDescription("Duration of database queries"),
		metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(1, 5, 10, 25, 50, 100, 250, 500, 1000),
	)
	if err != nil {
		log.Printf("[telemetry] Warning: failed to create db.queries.duration histogram: %v", err)
	}

	// Error Counter
	errorCounter, err = meter.Int64Counter("errors.total",
		metric.WithDescription("Total number of errors"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		log.Printf("[telemetry] Warning: failed to create errors.total counter: %v", err)
	}

	// Request Body Size
	requestBodySizeBytes, err = meter.Int64Histogram("http.client.request.body.size",
		metric.WithDescription("Size of HTTP request bodies in bytes"),
		metric.WithUnit("By"),
		metric.WithExplicitBucketBoundaries(100, 500, 1000, 5000, 10000, 50000, 100000, 500000, 1000000),
	)
	if err != nil {
		log.Printf("[telemetry] Warning: failed to create http.client.request.body.size histogram: %v", err)
	}

	// Response Body Size
	responseBodySizeBytes, err = meter.Int64Histogram("http.client.response.body.size",
		metric.WithDescription("Size of HTTP response bodies in bytes"),
		metric.WithUnit("By"),
		metric.WithExplicitBucketBoundaries(100, 500, 1000, 5000, 10000, 50000, 100000, 500000, 1000000),
	)
	if err != nil {
		log.Printf("[telemetry] Warning: failed to create http.client.response.body.size histogram: %v", err)
	}
}

// Shutdown gracefully shuts down OpenTelemetry instrumentation.
func Shutdown(ctx context.Context) error {
	log.Println("[telemetry] Shutting down OpenTelemetry...")

	var errs []error

	if tracerProvider != nil {
		if err := tracerProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown tracer provider: %w", err))
		}
	}

	if meterProvider != nil {
		if err := meterProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown meter provider: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("multiple shutdown errors: %v", errs)
	}

	log.Println("[telemetry] Shutdown complete")
	return nil
}

// config holds configuration options for telemetry initialization
type config struct {
	oTLPEndpoint string
	enabled      bool
}

// Option is a function that configures telemetry initialization.
type Option func(*config)

// WithOTLPEndpoint sets the OTLP endpoint.
func WithOTLPEndpoint(endpoint string) Option {
	return func(c *config) {
		c.oTLPEndpoint = endpoint
	}
}

// WithDisabled disables telemetry instrumentation.
func WithDisabled() Option {
	return func(c *config) {
		c.enabled = false
	}
}

// getEnvOrDefault returns the environment variable value or a default.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// GetTracer returns the global tracer.
func GetTracer() trace.Tracer {
	return tracer
}

// GetMeter returns the global meter.
func GetMeter() metric.Meter {
	return meter
}

// GetHTTPRequestCounter returns the HTTP request counter instrument.
func GetHTTPRequestCounter() metric.Int64Counter {
	return httpRequestCounter
}

// GetHTTPRequestDuration returns the HTTP request duration histogram.
func GetHTTPRequestDuration() metric.Float64Histogram {
	return httpRequestDuration
}

// GetTestDuration returns the test duration histogram.
func GetTestDuration() metric.Float64Histogram {
	return testDuration
}

// GetTestPassCounter returns the test pass counter.
func GetTestPassCounter() metric.Int64Counter {
	return testPassCounter
}

// GetTestFailCounter returns the test fail counter.
func GetTestFailCounter() metric.Int64Counter {
	return testFailCounter
}

// GetTestSkipCounter returns the test skip counter.
func GetTestSkipCounter() metric.Int64Counter {
	return testSkipCounter
}

// GetDBQueryCounter returns the DB query counter.
func GetDBQueryCounter() metric.Int64Counter {
	return dbQueryCounter
}

// GetDBQueryDuration returns the DB query duration histogram.
func GetDBQueryDuration() metric.Float64Histogram {
	return dbQueryDuration
}

// GetErrorCounter returns the error counter.
func GetErrorCounter() metric.Int64Counter {
	return errorCounter
}

// GetRequestBodySize returns the request body size histogram.
func GetRequestBodySize() metric.Int64Histogram {
	return requestBodySizeBytes
}

// GetResponseBodySize returns the response body size histogram.
func GetResponseBodySize() metric.Int64Histogram {
	return responseBodySizeBytes
}